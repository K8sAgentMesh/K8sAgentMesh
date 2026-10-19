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
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
)

// stubRuntime is a registry-backed fake whose Command runs a fixed shell
// snippet — enough to prove the HTTP surface, the event translation and the
// settle/close contract without any real coding-agent CLI.
type stubRuntime struct {
	typ     string
	command string
}

func (s stubRuntime) Type() string       { return s.typ }
func (s stubRuntime) CLIVersion() string { return "stub-v1" }
func (s stubRuntime) CredentialShape() agentruntime.CredentialShape {
	return agentruntime.ShapeAPIKey
}
func (s stubRuntime) Capabilities() a2a.Capabilities { return a2a.Capabilities{Streaming: true} }
func (s stubRuntime) DefaultModel() a2a.ModelInfo    { return a2a.ModelInfo{ID: "stub"} }
func (s stubRuntime) Command(lc agentruntime.LaunchContext) (agentruntime.ExecSpec, error) {
	return agentruntime.ExecSpec{
		Path:    "/bin/sh",
		Args:    []string{"-c", s.command},
		WorkDir: lc.WorkDir,
	}, nil
}

func init() {
	agentruntime.Register(stubRuntime{typ: "supervisor-stub", command: "echo hello-from-stub"})
	agentruntime.Register(stubRuntime{typ: "supervisor-stub-slow", command: "sleep 2"})
}

func stubServer(t *testing.T) *httptest.Server {
	t.Helper()
	rt, err := agentruntime.Get("supervisor-stub")
	if err != nil {
		t.Fatalf("get stub runtime: %v", err)
	}
	return httptest.NewServer(newHandler(rt, t.TempDir()))
}

func TestHealthAndReady(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()

	for _, path := range []string{"/health", "/ready"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestTaskSettlesOverNDJSON(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()

	body := `{"a2a_task_id":"t-1","envelope":{"input":"say hello"}}`
	resp, err := http.Post(srv.URL+"/task", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /task: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /task = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/x-ndjson") {
		t.Errorf("Content-Type = %q, want application/x-ndjson", ct)
	}

	// Decode the NDJSON stream as a2a.Event lines: first a working status,
	// a message from the stub CLI, and a terminal completed status that
	// ends the body (the settle contract).
	sc := bufio.NewScanner(resp.Body)
	var events []a2a.Event
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev a2a.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("bad NDJSON line %q: %v", line, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("got %d events, want >= 3 (working, message, completed): %+v", len(events), events)
	}
	if events[0].Type != a2a.EventStatus {
		t.Errorf("first event type = %q, want status", events[0].Type)
	}
	sp0, ok := events[0].Payload.(map[string]any)
	if !ok || sp0["state"] != string(a2a.TaskWorking) {
		t.Errorf("first status payload = %+v, want state working", events[0].Payload)
	}
	last := events[len(events)-1]
	if last.Type != a2a.EventStatus {
		t.Fatalf("last event type = %q, want status", last.Type)
	}
	spl, ok := last.Payload.(map[string]any)
	if !ok || spl["state"] != string(a2a.TaskCompleted) {
		t.Errorf("terminal payload = %+v, want state completed", last.Payload)
	}
	for _, ev := range events {
		if ev.A2ATaskID != "t-1" {
			t.Errorf("event %d task id = %q, want t-1", ev.Seq, ev.A2ATaskID)
		}
	}
}

func TestTaskRequiresTaskID(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/task", "application/json", strings.NewReader(`{"envelope":{}}`))
	if err != nil {
		t.Fatalf("POST /task: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /task without id = %d, want 400", resp.StatusCode)
	}
}

func TestTaskRejectsGarbage(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/task", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("POST /task: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("POST /task garbage = %d, want 400", resp.StatusCode)
	}
}

func TestOneTaskAtATime(t *testing.T) {
	rt, err := agentruntime.Get("supervisor-stub-slow")
	if err != nil {
		t.Fatalf("get slow stub runtime: %v", err)
	}
	srv := httptest.NewServer(newHandler(rt, t.TempDir()))
	defer srv.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := http.Post(srv.URL+"/task", "application/json",
			strings.NewReader(`{"a2a_task_id":"slow-1","envelope":{"input":"x"}}`))
		if err != nil {
			t.Errorf("slow POST: %v", err)
			return
		}
		// Drain to the terminal event: closing the body early would cancel
		// the request context and tear the running task down with it.
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// The slow task holds the slot; a concurrent task must get a legible
	// 409, not a serialized queue or an interleaved stream. Give the slow
	// task a beat to claim the slot first (a probe racing the holder's
	// first request would legitimately win it).
	time.Sleep(300 * time.Millisecond)
	resp, err := http.Post(srv.URL+"/task", "application/json",
		strings.NewReader(`{"a2a_task_id":"fast-1","envelope":{"input":"y"}}`))
	if err != nil {
		t.Fatalf("concurrent POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("concurrent POST = %d, want 409", resp.StatusCode)
	}
	<-done
}
