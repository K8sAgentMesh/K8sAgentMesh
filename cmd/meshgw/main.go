/*
Copyright 2026 The K8sAgentMesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Command meshgw is the AgentMesh gateway server — the runnable entrypoint of
// the dedicated project image (ISI-6205, unblocking the ISI-6103 build
// hardening).
//
// What the container's job is: run the per-pod PID1 process-manager
// (mesh.Gateway, chapter §2.3 D1.2). The gateway holds a pool of standing
// coding-agent runtime processes behind the one RuntimeAdapter seam, enforces
// the §6 lifecycle (spawn / idle-sweep / hibernate / stop / recreate-on-demand)
// and the one-process-one-task guard, and registers its live endpoint set to a
// Supervisor ingress on a heartbeat. The gateway is the only standing,
// long-lived service this [generic] library defines — the Supervisor ingress
// is the host-owned control-plane side, and a one-shot CLI would not be a
// service — so the gateway is the honest subject of a dedicated image.
//
// The runtime served is goose (adapter #4, ISI-6197) by default, wired behind
// RuntimeAdapter so the gateway never names it. Select another built-in runtime
// (vibe / opencode / codex) via MESHGW_RUNTIME; adding one is a new spec, no
// change here.
//
// HTTP control surface:
//
//	GET  /healthz  liveness — 200 once the process serves. Deliberately
//	              independent of the pool: a gateway whose runtime substrate
//	              (the shim + CLI binaries, layered by ISI-6103) is not yet
//	              present stays LIVE but not READY, the standard k8s posture,
//	              instead of crash-looping. This is also what lets the image
//	              build and the entrypoint start today, while the repo is still
//	              a library carrying no shim binary.
//	GET  /ready   readiness — 200 when the pool can serve a route
//	              (mesh.Gateway.Ready), 503 otherwise.
//	POST /task    A2A task delivery — decodes an a2a.Task, routes it to a
//	              standing process (?target=<endpoint_id> selects one; empty
//	              picks the LRU idle), and streams the task's events back as
//	              NDJSON, the same wire the shim supervisor /task serves.
//
// All configuration is environment-driven (12-factor); see the env* constants.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

// version is stamped at build time via -ldflags "-X main.version=...". It is
// surfaced by the -version flag and logged at boot for provenance.
var version = "dev"

// Environment contract. Every knob has a safe default so an image boots with
// no env set (and serves /healthz) — the build-only acceptance path.
const (
	envListenAddr  = "MESHGW_ADDR"          // HTTP listen address (default :8080)
	envRuntime     = "MESHGW_RUNTIME"       // runtime flavor: goose|opencode|codex (default goose)
	envTeam        = "MESHGW_TEAM"          // pool key: team (required to register)
	envRole        = "MESHGW_ROLE"          // pool key: role (required to register)
	envPod         = "MESHGW_POD"           // pod identity on registrations (default HOSTNAME)
	envMin         = "MESHGW_MIN"           // standing process floor (default 1)
	envMax         = "MESHGW_MAX"           // saturation ceiling (default = min)
	envModel       = "MESHGW_MODEL"         // envelope model override (default: runtime default)
	envRegisterURL = "MESHGW_REGISTER_URL"  // Supervisor ingress base URL; empty disables registration
	envAdvertise   = "MESHGW_ADVERTISE_URL" // gateway's own routable URL advertised to the Supervisor
	envShimBin     = "MESHGW_SHIM_BIN"      // shim executable LocalHost execs (default "shim" on PATH)
	envRuntimeEnv  = "MESHGW_RUNTIME_ENV"   // the launch-env key the shim reads to pick its flavor (default KSQUAD_RUNTIME_TYPE)
)

func main() {
	for _, a := range os.Args[1:] {
		if a == "-version" || a == "--version" {
			fmt.Printf("meshgw %s\n", version)
			return
		}
	}

	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("meshgw %s starting", version)

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("meshgw: config: %v", err)
	}

	gw, err := mesh.NewGateway(cfg.gateway)
	if err != nil {
		log.Fatalf("meshgw: build gateway: %v", err)
	}

	// Root context cancelled on SIGINT/SIGTERM — it governs the pool's
	// lifecycle/registration loops and the graceful shutdown below.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           newMux(gw),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Boot the pool in the background. A start failure (e.g. no shim binary in
	// this build-only image) is logged, not fatal: /healthz stays 200 and
	// /ready reports not-ready until the substrate is present — the live-but-
	// not-ready posture, never a crash loop.
	go func() {
		if err := gw.Start(ctx); err != nil {
			log.Printf("meshgw: pool start failed (serving not-ready): %v", err)
		} else {
			log.Printf("meshgw: pool started (runtime=%s min=%d max=%d)", cfg.runtime, cfg.gateway.Min, cfg.gateway.Max)
		}
	}()

	srvErr := make(chan error, 1)
	go func() {
		log.Printf("meshgw: listening on %s", cfg.addr)
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-srvErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("meshgw: serve: %v", err)
		}
	case <-ctx.Done():
		log.Printf("meshgw: shutdown signal received")
	}

	// Graceful drain: stop the pool's children, then the HTTP server.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := gw.Stop(shutdownCtx); err != nil {
		log.Printf("meshgw: gateway stop: %v", err)
	}
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("meshgw: http shutdown: %v", err)
	}
	log.Printf("meshgw: stopped")
}

// config is the resolved boot configuration.
type config struct {
	addr    string
	runtime string
	gateway mesh.GatewayConfig
}

// loadConfig reads the environment contract into a GatewayConfig, building the
// LocalHost substrate and the selected RuntimeAdapter.
func loadConfig() (config, error) {
	addr := getenv(envListenAddr, ":8080")
	runtimeType := getenv(envRuntime, "goose")
	runtimeEnvKey := getenv(envRuntimeEnv, "KSQUAD_RUNTIME_TYPE")

	minProc, err := getenvInt(envMin, 1)
	if err != nil {
		return config{}, err
	}
	maxProc, err := getenvInt(envMax, minProc)
	if err != nil {
		return config{}, err
	}

	pod := getenv(envPod, "")
	if pod == "" {
		pod, _ = os.Hostname()
	}

	host := &mesh.LocalHost{
		BinPath: getenv(envShimBin, "shim"),
		Args:    []string{"supervisor"},
	}

	adapter, err := newAdapter(runtimeType, runtimeEnvKey, host)
	if err != nil {
		return config{}, err
	}

	return config{
		addr:    addr,
		runtime: runtimeType,
		gateway: mesh.GatewayConfig{
			Adapter: adapter,
			Envelope: mesh.Envelope{
				RuntimeType: runtimeType,
				Model:       getenv(envModel, ""),
			},
			Min:          minProc,
			Max:          maxProc,
			Pod:          pod,
			Team:         getenv(envTeam, ""),
			Role:         getenv(envRole, ""),
			RegisterURL:  strings.TrimRight(getenv(envRegisterURL, ""), "/"),
			AdvertiseURL: getenv(envAdvertise, ""),
		},
	}, nil
}

// newAdapter builds the selected runtime adapter behind the RuntimeAdapter
// seam. The runtime profiles are the honest, spike-proven capability sets each
// adapter's tests inject; a host with its own declarative registry supplies
// richer specs, but the gateway seam is identical.
func newAdapter(runtimeType, runtimeEnvKey string, host mesh.PlatformHost) (mesh.RuntimeAdapter, error) {
	switch runtimeType {
	case "vibe":
		return vibeAdapter(runtimeEnvKey, host)
	case "goose":
		return mesh.NewGooseAdapter(host, gooseSpec(runtimeEnvKey))
	case "opencode":
		return mesh.NewOpenCodeAdapter(host, baseSpec("opencode", runtimeEnvKey))
	case "codex":
		return mesh.NewCodexAdapter(host, baseSpec("codex", runtimeEnvKey))
	default:
		return nil, fmt.Errorf("unknown %s=%q (want vibe|goose|opencode|codex)", envRuntime, runtimeType)
	}
}

// vibeAdapter builds the registry-backed vibe adapter (ISI-6217 gap 1's
// vanilla story): the spec values (pinned CLI revision, capabilities,
// default model, mistral credential type) are single-sourced from the
// agentruntime registry — the same source of truth cmd/mesh-gateway reads —
// instead of a second hand-maintained profile.
func vibeAdapter(runtimeEnvKey string, host mesh.PlatformHost) (mesh.RuntimeAdapter, error) {
	rt, err := agentruntime.Get(agentruntime.RuntimeTypeVibe)
	if err != nil {
		return nil, err
	}
	return mesh.NewVibeAdapter(host, mesh.RuntimeSpec{
		Type:           rt.Type(),
		CLIVersion:     rt.CLIVersion(),
		Capabilities:   rt.Capabilities(),
		DefaultModel:   rt.DefaultModel(),
		CredentialType: agentruntime.CredentialTypeMistral,
		SchemaVersion:  a2a.DefaultSchemaVersion,
		Protocol:       a2a.ProtocolVersions{A2A: "0.2.0", MCP: "2025-06-18"},
		RuntimeTypeEnv: runtimeEnvKey,
	})
}

// gooseSpec is the goose runtime profile (ISI-6197): the spike-proven surface —
// streaming, tool calls, BYO OpenAI-compatible endpoint, NO interactive
// approval (headless GOOSE_MODE=auto) — with the CLI pin and api-key credential
// shape the Phase A spike locked.
func gooseSpec(runtimeEnvKey string) mesh.RuntimeSpec {
	s := baseSpec("goose", runtimeEnvKey)
	s.CLIVersion = "1.55.0"
	s.Capabilities = a2a.Capabilities{Streaming: true, ToolCalls: true, BYOModelEndpoint: true}
	s.DefaultModel = a2a.ModelInfo{ID: "qwen3.5:35b", ContextWindow: 131072}
	s.CredentialType = "apikey"
	return s
}

// baseSpec is the shared runtime spec skeleton: the flavor name, the launch-env
// contract the shim reads to pick its baked-in runtime, and streaming (the
// mandatory A2A V2 capability).
func baseSpec(runtimeType, runtimeEnvKey string) mesh.RuntimeSpec {
	return mesh.RuntimeSpec{
		Type:           runtimeType,
		Capabilities:   a2a.Capabilities{Streaming: true},
		RuntimeTypeEnv: runtimeEnvKey,
	}
}

// newMux wires the gateway's HTTP control surface.
func newMux(gw *mesh.Gateway) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		if gw.Ready() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready\n"))
			return
		}
		http.Error(w, "pool not ready", http.StatusServiceUnavailable)
	})
	mux.HandleFunc("POST /task", taskHandler(gw))
	return mux
}

// taskHandler decodes an a2a.Task and streams its routed event sequence back as
// NDJSON — one JSON event per line, flushed as it arrives — mirroring the shim
// supervisor /task wire the mesh httpRouter already speaks.
func taskHandler(gw *mesh.Gateway) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		var task a2a.Task
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&task); err != nil {
			http.Error(w, fmt.Sprintf("decode task: %v", err), http.StatusBadRequest)
			return
		}
		if task.A2ATaskID == "" {
			http.Error(w, "task requires a2a_task_id", http.StatusBadRequest)
			return
		}

		stream, err := gw.Route(r.Context(), r.URL.Query().Get("target"), task)
		if err != nil {
			// Busy / saturated are legible 409s, matching the gateway's own
			// one-process-one-task refusal; everything else is a 502.
			if errors.Is(err, mesh.ErrProcessBusy) || errors.Is(err, mesh.ErrNoIdleProcess) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = stream.Close() }()

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		enc := json.NewEncoder(w)
		for ev := range stream.Events() {
			if err := enc.Encode(ev); err != nil {
				return // follower gone or write failed; Close drains the task
			}
			flusher.Flush()
		}
	}
}

// getenv returns the env value or def when unset/empty.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// getenvInt parses an integer env value, returning def when unset/empty.
func getenvInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: not an integer: %w", key, v, err)
	}
	return n, nil
}
