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

// Command mesh-supervisor is the mesh-owned standing-process supervisor —
// the vanilla replacement for the host project's `shim supervisor`
// (ISI-6217 gap 1): mesh.LocalHost execs it (default BinPath resolution
// honors MESHGW_SHIM_BIN) once per standing slot with
// KSQUAD_SUPERVISOR_ADDR stamped, and it serves the A2A surface the mesh
// httpRouter and ProbeReady already speak:
//
//	GET  /health  liveness — the supervisor process is serving.
//	GET  /ready   readiness — 200 once the listener is up (the vanilla
//	              surface has no credential handshake; gate readiness on
//	              liveness).
//	POST /task    task ingest — decodes one a2a.Task, drives it through
//	              the agentruntime registry (internal/taskrun core, the
//	              same execution engine cmd/agentruntime runs), and
//	              streams the task's a2a.Event sequence back over the
//	              response body as NDJSON, closing exactly once at the
//	              terminal status (the ISI-4224 settle bar).
//
// This is also the chart's task-ingest surface (ISI-6217 gap 2): the
// agentruntime Deployment runs this binary fronted by a Service, so a
// deployed pod answers POST /task directly instead of EOF-ing on a
// stdin nobody attached.
//
// The runtime flavor is stamped into the pod env (KSQUAD_RUNTIME_TYPE);
// credentials NEVER ride the task JSON — they are read from the process env
// (MISTRAL_API_KEY / OPENAI_API_KEY). One task at a time per process (the
// gateway's one-process-one-task guard is caller-side; the supervisor
// enforces it server-side too, answering 409 while a task is streaming).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
	"github.com/K8sAgentMesh/K8sAgentMesh/internal/taskrun"
)

const (
	// envSupervisorAddr is the listen address LocalHost stamps per standing
	// slot so N processes in one pod bind distinct ports (the
	// k8squad-supervisor contract, unchanged).
	envSupervisorAddr = "KSQUAD_SUPERVISOR_ADDR"
	// envRuntimeType is the standing-process flavor contract.
	envRuntimeType = "KSQUAD_RUNTIME_TYPE"
	// envWorkdirRoot is the parent directory per-task workdirs are created
	// under (default the system temp dir).
	envWorkdirRoot = "MESH_SUPERVISOR_WORKDIR"
	// defaultAddr matches the k8squad shim supervisor default.
	defaultAddr = ":8080"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	// Address resolution mirrors the k8squad shim supervisor: env wins; a
	// positional arg is honored only when the env is unset. LocalHost always
	// passes args[0]=="supervisor" (its default Args), which is stripped so
	// both `mesh-supervisor` and `mesh-supervisor supervisor` boot identically.
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "supervisor" {
		args = args[1:]
	}
	addr := defaultAddr
	if v := os.Getenv(envSupervisorAddr); v != "" {
		addr = v
	} else if len(args) > 0 && args[0] != "" {
		addr = args[0]
	}

	runtimeType := os.Getenv(envRuntimeType)
	if runtimeType == "" {
		fmt.Fprintln(os.Stderr, "mesh-supervisor:", envRuntimeType, "is required (the standing-process flavor contract)")
		os.Exit(1)
	}
	rt, err := agentruntime.Get(runtimeType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mesh-supervisor:", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           newHandler(rt, os.Getenv(envWorkdirRoot)),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// A supervisor that cannot serve is a dead standing slot: fail loudly so
	// the gateway observes the launch failure (never a silent zombie).
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mesh-supervisor: listen:", err)
		os.Exit(1)
	}
	// #nosec G706 -- addr/runtimeType are operator-set boot configuration
	// (pod env; LocalHost stamps the host:port), never task-channel input:
	// no log-injection surface. net.Listen below rejects a malformed addr
	// before anything serves.
	log.Printf("mesh-supervisor: serving on %s (runtime=%s)", addr, runtimeType)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "mesh-supervisor: serve:", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		// LocalHost Close SIGTERMs the process group: drain the in-flight
		// task stream, then exit.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("mesh-supervisor: shutdown: %v", err)
		}
	}
}

// newHandler wires the supervisor's HTTP surface. workdirRoot is the
// parent for per-task workdirs ("" = the system default).
func newHandler(rt agentruntime.Runtime, workdirRoot string) http.Handler {
	s := &supervisor{rt: rt, workdirRoot: workdirRoot}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("POST /task", s.handleTask)
	return mux
}

// supervisor is the standing HTTP surface state: the resolved runtime, the
// per-task workdir parent, and the one-task-at-a-time guard.
type supervisor struct {
	rt          agentruntime.Runtime
	workdirRoot string

	mu   sync.Mutex // one task at a time (server-side half of the exclusivity guard)
	seq  atomic.Uint64
	busy bool
}

func (s *supervisor) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *supervisor) handleReady(w http.ResponseWriter, _ *http.Request) {
	// The vanilla surface has no credential handshake: readiness is
	// liveness (the listener answers). Readiness gating on credentials is a
	// host-project concern (the k8squad Bind handshake), not a mesh one.
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready\n"))
}

// handleTask decodes one a2a.Task, runs it through the shared task core and
// streams the translated a2a.Event NDJSON sequence back, flushed per line,
// closing exactly once at the terminal status.
func (s *supervisor) handleTask(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	var task a2a.Task
	// The envelope rides the body (never argv): allow the same generous
	// ceiling the stdio path buffers per line.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("decode task: %v", err), http.StatusBadRequest)
		return
	}
	if task.A2ATaskID == "" {
		http.Error(w, "task requires a2a_task_id", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		// Legible exclusivity refusal, matching the gateway's own
		// one-process-one-task 409 semantics.
		http.Error(w, "supervisor busy: one task at a time per process", http.StatusConflict)
		return
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()

	workDir, err := os.MkdirTemp(s.workdirRoot, "mesh-task-")
	if err != nil {
		http.Error(w, fmt.Sprintf("workdir: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	emit := func(ev a2a.Event) {
		if err := enc.Encode(ev); err != nil {
			return // follower gone; the task run continues to its terminal event
		}
		_ = rc.Flush()
	}
	nextSeq := func() uint64 { return s.seq.Add(1) }
	event := func(typ a2a.EventType, payload any) a2a.Event {
		return a2a.Event{Seq: nextSeq(), A2ATaskID: task.A2ATaskID, TS: time.Now().UTC(), Type: typ, Payload: payload}
	}

	// submitted → working before the CLI spawns, so a slow cold start still
	// reads as live on the stream (spec §3.1).
	emit(event(a2a.EventStatus, a2a.StatusPayload{State: a2a.TaskWorking}))

	taskrun.Run(r.Context(), s.rt, taskrun.Request{
		ID:         task.A2ATaskID,
		Envelope:   task.Envelope,
		ModelRoute: task.ModelRoute,
		WorkDir:    workDir,
	}, func(ev taskrun.Event) {
		switch ev.Kind {
		case "message":
			emit(event(a2a.EventMessage, a2a.MessagePayload{Role: ev.Role, Text: ev.Content, Trust: "untrusted"}))
		case "tool":
			emit(event(a2a.EventTool, a2a.ToolPayload{Name: ev.Name, Phase: "start"}))
		case "completed":
			reason := ""
			if ev.Exit != 0 {
				reason = fmt.Sprintf("exit %d", ev.Exit)
			}
			emit(event(a2a.EventStatus, a2a.StatusPayload{State: a2a.TaskCompleted, Reason: reason}))
		case "failed":
			emit(event(a2a.EventStatus, a2a.StatusPayload{State: a2a.TaskFailed, Reason: ev.Error}))
		}
	})
	// The body closes here — exactly once, at the terminal event: the
	// httpStream settle contract.
}
