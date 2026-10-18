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

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

// loadConfig with an empty environment must yield a valid, buildable gateway
// config defaulting to the goose runtime — the build-only boot path the image
// relies on (no env set, no shim binary present).
func TestLoadConfigDefaultsToGoose(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig with empty env: %v", err)
	}
	if cfg.runtime != "goose" {
		t.Fatalf("default runtime = %q, want goose", cfg.runtime)
	}
	if cfg.addr != ":8080" {
		t.Fatalf("default addr = %q, want :8080", cfg.addr)
	}
	if cfg.gateway.Adapter == nil {
		t.Fatal("loadConfig must build a RuntimeAdapter")
	}
	// The config must assemble into a real gateway without spawning anything.
	if _, err := mesh.NewGateway(cfg.gateway); err != nil {
		t.Fatalf("NewGateway from default config: %v", err)
	}
}

// ISI-6217: vibe is a first-class gateway runtime, registry-backed so the
// spec values (pinned CLI, capabilities, default model) stay single-sourced
// with cmd/mesh-gateway and the toolchain image pin.
func TestNewAdapterVibe(t *testing.T) {
	ad, err := newAdapter("vibe", "KSQUAD_RUNTIME_TYPE", &mesh.LocalHost{})
	if err != nil {
		t.Fatalf("newAdapter(vibe): %v", err)
	}
	if ad == nil {
		t.Fatal("newAdapter(vibe) returned a nil adapter")
	}
}

func TestNewAdapterRejectsUnknownRuntime(t *testing.T) {
	if _, err := newAdapter("nope", "KSQUAD_RUNTIME_TYPE", &mesh.LocalHost{}); err == nil {
		t.Fatal("newAdapter must reject an unknown runtime flavor")
	}
}

// /healthz is liveness and must be 200 regardless of pool readiness; /ready
// reflects the pool and is 503 on an unstarted gateway (no standing process) —
// the live-but-not-ready posture the entrypoint depends on.
func TestMuxHealthzIsLiveWhileNotReady(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	gw, err := mesh.NewGateway(cfg.gateway)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	mux := newMux(gw)

	healthz := httptest.NewRecorder()
	mux.ServeHTTP(healthz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthz.Code != http.StatusOK {
		t.Fatalf("/healthz = %d, want 200", healthz.Code)
	}

	ready := httptest.NewRecorder()
	mux.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("/ready (unstarted pool) = %d, want 503", ready.Code)
	}
}

// A /task POST with no a2a_task_id is a 400 before any routing is attempted.
func TestTaskHandlerRejectsMissingID(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	gw, err := mesh.NewGateway(cfg.gateway)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader(`{"work_item_id":"w1"}`))
	newMux(gw).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("/task without a2a_task_id = %d, want 400", rec.Code)
	}
}
