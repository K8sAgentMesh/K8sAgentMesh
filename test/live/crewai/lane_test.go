//go:build crewai_lane

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

// The live CrewAI lane (ISI-6547 phase 4): the real crewaiRouter against a
// real pinned crewai[a2a]==1.15.28 server — a real Crew kickoff through the
// litellm lane — instead of the fake-A2A httptest suite. Proves, on the
// default JSON-RPC binding the spike locked:
//
//   - a live crew task routes via the mesh wire and settles completed, with
//     the crew's artifact riding EventArtifactRef (marker-echoed so the
//     output provably flowed model→crew→A2A→router);
//   - C1 for real: a torn SSE tail followed by a re-route on the same
//     a2a_task_id reattaches via tasks/get — the crew runs exactly once;
//   - CancelTask maps: Stream.Close() lands tasks/cancel server-side;
//   - the agent card is served at /.well-known/agent-card.json and POST /a2a
//     enforces the AUTH_TOKEN bearer (SimpleTokenAuth semantics).
//
// Gated twice: the crewai_lane build tag keeps it out of default builds, and
// CREWAI_LANE=1 + CREWAI_LANE_PYTHON keep a tagged run from failing on a
// machine without the pinned stack. lane.sh provisions the venv and runs it.
package crewailane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

const (
	laneToken  = "lane-token"
	probeFixed = "wire-probe-final-answer"
)

func TestLiveCrewaiLane(t *testing.T) {
	if os.Getenv("CREWAI_LANE") != "1" {
		t.Skip("set CREWAI_LANE=1 (see test/live/crewai/README.md)")
	}
	py := os.Getenv("CREWAI_LANE_PYTHON")
	if py == "" {
		t.Skip("set CREWAI_LANE_PYTHON to a python with crewai[a2a]==1.15.28, a2a-sdk~=0.3.10, starlette and uvicorn (lane.sh provisions one)")
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	logDir := t.TempDir()

	mockPort := freePort(t)
	crewPort := freePort(t)

	mockEnv := []string{"HOLD_SECONDS=6"}
	startProc(t, py, filepath.Join(dir, "mock_openai.py"), logDir, "mock.log", mockEnv,
		"--port", fmt.Sprint(mockPort))
	waitReach(t, fmt.Sprintf("http://127.0.0.1:%d/healthz", mockPort), 30*time.Second, "mock LLM")

	crewEnv := []string{
		"AUTH_TOKEN=" + laneToken,
		"OPENAI_API_KEY=lane-dummy",
		"CREWAI_TELEMETRY_OPT_OUT=true",
	}
	startProc(t, py, filepath.Join(dir, "crew_server.py"), logDir, "crew.log", crewEnv,
		"--port", fmt.Sprint(crewPort),
		"--llm-url", fmt.Sprintf("http://127.0.0.1:%d/v1", mockPort))
	base := fmt.Sprintf("http://127.0.0.1:%d", crewPort)
	waitReach(t, base+"/.well-known/agent-card.json", 180*time.Second, "crew server (crewai import is slow)")

	// The card contract: served at the well-known path, JSON, the crew's own.
	card := getJSON(t, base+"/.well-known/agent-card.json")
	if card["name"] != "lane-crew" {
		t.Fatalf("agent card name = %v, want lane-crew", card["name"])
	}
	if pv, _ := card["protocolVersion"].(string); pv != "0.3.0" {
		t.Fatalf("agent card protocolVersion = %v, want 0.3.0", card["protocolVersion"])
	}

	statsURL := base + "/lane/stats"
	kickoffs := func() int { return int(getJSON(t, statsURL)["kickoffs"].(float64)) }
	cancels := func() int { return int(getJSON(t, statsURL)["cancels"].(float64)) }

	// The bearer contract: POST /a2a without the token is refused.
	if code := postStatus(t, base+"/a2a", ""); code != http.StatusUnauthorized {
		t.Fatalf("POST /a2a without bearer = %d, want 401 (SimpleTokenAuth/AUTH_TOKEN)", code)
	}

	r := mesh.NewCrewaiRouter(nil, laneToken)
	ctx := context.Background()

	t.Run("HappyPath", func(t *testing.T) {
		task := a2a.Task{
			A2ATaskID: "lane-live-1",
			Envelope: a2a.Envelope{
				SystemContext: "You are the live-lane crew.",
				Input:         "marker: LIVE1 — produce the probe phrase.",
			},
		}
		s, err := r.Route(ctx, base, task)
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		evs := collect(t, s, 90*time.Second)
		assertGapFree(t, evs, 1)
		assertTerminal(t, s, a2a.TaskCompleted)
		if n := kickoffs(); n != 1 {
			t.Fatalf("kickoffs = %d, want exactly 1", n)
		}
		if !sawMessageWith(t, evs, probeFixed+" LIVE1") {
			t.Fatalf("no event carried the marker-echoed crew output %q (model→crew→A2A→router data flow)", probeFixed+" LIVE1")
		}
		if !sawArtifact(t, evs) {
			t.Fatal("no EventArtifactRef — the crew output must ride the artifact surface")
		}
	})

	t.Run("C1RerouteReattaches", func(t *testing.T) {
		// The HOLD marker makes the mock model hold 6s: the task is still
		// in flight when a second follower re-routes onto it. The first
		// follower stays attached (no Close — Close is the cancel path, a
		// separate contract), mirroring the fake-suite C1 scenarios.
		task := a2a.Task{
			A2ATaskID: "lane-live-c1",
			Envelope:  a2a.Envelope{Input: "HOLD this one — reattach probe marker: RE1"},
		}
		first, err := r.Route(ctx, base, task)
		if err != nil {
			t.Fatalf("first Route: %v", err)
		}
		if _, ok := <-first.Events(); !ok {
			t.Fatal("first stream closed before any event — the working frame never arrived")
		}
		var firstDrained []a2a.Event
		k := kickoffs()
		second, err := r.Route(ctx, base, task) // re-route, same a2a_task_id
		if err != nil {
			t.Fatalf("re-route: %v", err)
		}
		evs := collect(t, second, 90*time.Second)
		if len(evs) == 0 {
			t.Fatal("reattached stream produced no events")
		}
		if evs[0].Seq < 2 {
			t.Fatalf("reattach first seq = %d, want continuation ≥2 (gap-free C4 across reattach)", evs[0].Seq)
		}
		assertTerminal(t, second, a2a.TaskCompleted)
		if n := kickoffs(); n != k {
			t.Fatalf("kickoffs = %d after re-route, want %d — re-route must reattach, never re-send (C1)", n, k)
		}
		// The artifact event is delivered exactly once, on whichever
		// follower's transport is live when it lands (shared task state) —
		// the union of both followers must carry it and the marker-echoed
		// crew output.
		drainDeadline := time.After(10 * time.Second)
	collectFirst:
		for {
			select {
			case ev, ok := <-first.Events():
				if !ok {
					break collectFirst
				}
				firstDrained = append(firstDrained, ev)
			case <-drainDeadline:
				break collectFirst
			}
		}
		union := append(append([]a2a.Event{}, evs...), firstDrained...)
		if !sawMessageWith(t, union, probeFixed+" RE1") {
			t.Fatalf("neither follower saw the marker-echoed crew output %q (union=%v)", probeFixed+" RE1", summarize(union))
		}
		if !sawArtifact(t, union) {
			t.Fatal("no EventArtifactRef on either follower — the crew output must ride the artifact surface")
		}
		_ = first.Close() // both followers done; terminal, so no cancel fires
	})

	t.Run("CloseMapsToCancel", func(t *testing.T) {
		task := a2a.Task{
			A2ATaskID: "lane-live-cancel",
			Envelope:  a2a.Envelope{Input: "HOLD this one too — cancel probe"},
		}
		s, err := r.Route(ctx, base, task)
		if err != nil {
			t.Fatalf("Route: %v", err)
		}
		if _, ok := <-s.Events(); !ok {
			t.Fatal("stream closed before the working frame arrived")
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if cancels() >= 1 {
				return
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("tasks/cancel never landed server-side (cancels=%d) — logs in %s", cancels(), logDir)
	})
}

// --- lane plumbing -------------------------------------------------------

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func startProc(t *testing.T, py, script, logDir, logName string, env []string, args ...string) {
	t.Helper()
	logFile, err := os.Create(filepath.Join(logDir, logName))
	if err != nil {
		t.Fatalf("create %s: %v", logName, err)
	}
	cmd := exec.Command(py, append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", script, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = logFile.Close()
	})
}

func waitReach(t *testing.T, url string, timeout time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:gosec // lane fixture URL
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s never became reachable at %s within %s", what, url, timeout)
}

func getJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec // lane fixture URL
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("GET %s: bad json %q: %v", url, string(body), err)
	}
	return out
}

func postStatus(t *testing.T, url, bearer string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(
		`{"jsonrpc":"2.0","id":0,"method":"message/stream","params":{"message":{"role":"user","parts":[{"kind":"text","text":"probe"}],"messageId":"auth-probe","kind":"message"}}}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode
}

func collect(t *testing.T, s mesh.Stream, timeout time.Duration) []a2a.Event {
	t.Helper()
	var out []a2a.Event
	deadline := time.After(timeout)
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			t.Fatalf("stream did not settle within %s; events so far: %v", timeout, summarize(out))
		}
	}
}

func assertGapFree(t *testing.T, evs []a2a.Event, first uint64) {
	t.Helper()
	for i, ev := range evs {
		if want := first + uint64(i); ev.Seq != want {
			t.Fatalf("event %d seq = %d, want %d (gap-free C4)", i, ev.Seq, want)
		}
	}
}

func assertTerminal(t *testing.T, s mesh.Stream, want a2a.TaskState) {
	t.Helper()
	if st := s.Status(); st.State != want {
		t.Fatalf("terminal status = %v, want %v", st.State, want)
	}
}

func sawMessageWith(t *testing.T, evs []a2a.Event, needle string) bool {
	t.Helper()
	for _, ev := range evs {
		if ev.Type != a2a.EventMessage {
			continue
		}
		mp, ok := ev.Payload.(a2a.MessagePayload)
		if !ok {
			continue
		}
		if strings.Contains(mp.Text, needle) {
			return true
		}
	}
	return false
}

func sawArtifact(t *testing.T, evs []a2a.Event) bool {
	t.Helper()
	for _, ev := range evs {
		if ev.Type == a2a.EventArtifactRef {
			if ref, ok := ev.Payload.(a2a.ArtifactRef); ok {
				if ref.SHA256 != "" {
					t.Fatalf("artifact sha256 = %q, want empty — never fabricated (NFR-2)", ref.SHA256)
				}
			}
			return true
		}
	}
	return false
}

func summarize(evs []a2a.Event) []string {
	out := make([]string, 0, len(evs))
	for _, e := range evs {
		out = append(out, fmt.Sprintf("%d/%s", e.Seq, e.Type))
	}
	return out
}
