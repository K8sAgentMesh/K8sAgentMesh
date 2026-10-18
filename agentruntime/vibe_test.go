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

package agentruntime

import (
	"strings"
	"testing"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
)

func TestVibeRegistryEntry(t *testing.T) {
	rt, err := Get(RuntimeTypeVibe)
	if err != nil {
		t.Fatalf("Get(vibe): %v", err)
	}
	if got := rt.CLIVersion(); got != "py-v0.3.0" {
		t.Errorf("CLIVersion = %q, want py-v0.3.0", got)
	}
	if got := rt.Type(); got != RuntimeTypeVibe {
		t.Errorf("Type = %q, want %q", got, RuntimeTypeVibe)
	}
	if rt.CredentialShape() != ShapeAPIKey {
		t.Errorf("CredentialShape = %q, want api-key", rt.CredentialShape())
	}
	dm := rt.DefaultModel()
	if dm.ID != "mistral-large-latest" || dm.ContextWindow != 131072 {
		t.Errorf("DefaultModel = %+v, want mistral-large-latest/131072", dm)
	}
	caps := rt.Capabilities()
	if !caps.Streaming || !caps.ToolCalls || caps.InteractivePrompt {
		t.Errorf("Capabilities = %+v, want streaming+toolcalls, headless (no interactive)", caps)
	}
	if !caps.BYOModelEndpoint {
		t.Errorf("Capabilities.BYOModelEndpoint = false, want true (MISTRAL_BASE_URL lane)")
	}
}

func TestVibeCommandShape(t *testing.T) {
	rt := vibe{}
	spec, err := rt.Command(LaunchContext{
		Envelope: a2a.Envelope{SystemContext: "sys", Input: "do the thing"},
		Model:    "mistral-small-latest",
		WorkDir:  "/wt/42",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if spec.Path != "vibe" {
		t.Fatalf("Path = %q, want vibe", spec.Path)
	}
	wantArgs := strings.Join([]string{
		"-p", "--output", "streaming", "--auto-approve",
		"--workdir", "/wt/42", "--max-turns", "64", "--trust",
	}, " ")
	if got := strings.Join(spec.Args, " "); got != wantArgs {
		t.Errorf("Args = %q, want %q", got, wantArgs)
	}
	if spec.Stdin != "sys\n\ndo the thing" {
		t.Errorf("Stdin = %q, want reassembled envelope prompt", spec.Stdin)
	}
	if got := envGet(spec.Env, "VIBE_ACTIVE_MODEL"); got != "mistral-small-latest" {
		t.Errorf("VIBE_ACTIVE_MODEL = %q, want the routed model (GrowthBook bug mitigation)", got)
	}
	if envHas(spec.Env, "MISTRAL_API_KEY") {
		t.Errorf("MISTRAL_API_KEY set with no credential and no route; want empty lane")
	}
}

func TestVibeCommandModelRoute(t *testing.T) {
	rt := vibe{}
	spec, err := rt.Command(LaunchContext{
		Envelope:   a2a.Envelope{Input: "work"},
		ModelRoute: a2a.ModelRoute{Endpoint: "http://ollama:11434/v1", Model: "qwen3.5:122B"},
		WorkDir:    "/wt/7",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got := envGet(spec.Env, "VIBE_ACTIVE_MODEL"); got != "qwen3.5:122B" {
		t.Errorf("VIBE_ACTIVE_MODEL = %q, want the BYO route model", got)
	}
	if got := envGet(spec.Env, "MISTRAL_BASE_URL"); got != "http://ollama:11434/v1" {
		t.Errorf("MISTRAL_BASE_URL = %q, want the BYO endpoint", got)
	}
	if got := envGet(spec.Env, "MISTRAL_API_KEY"); got != "ollama" {
		t.Errorf("MISTRAL_API_KEY = %q, want the zero-credential ollama placeholder", got)
	}
}

func TestVibeCommandRouteTokenWins(t *testing.T) {
	rt := vibe{}
	spec, err := rt.Command(LaunchContext{
		Envelope:   a2a.Envelope{Input: "work"},
		Credential: "user-key",
		ModelRoute: a2a.ModelRoute{Endpoint: "https://byo/v1", Token: "route-key", Model: "m"},
		WorkDir:    "/wt",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got := envGet(spec.Env, "MISTRAL_API_KEY"); got != "route-key" {
		t.Errorf("MISTRAL_API_KEY = %q, want route token to win (exactly one key emitted)", got)
	}
}

func TestParseVibeLine(t *testing.T) {
	events := parseVibeLine(`{"type":"assistant","text":"hello there"}`)
	if len(events) != 1 || events[0].Kind != a2a.EventMessage {
		t.Fatalf("assistant line: %+v", events)
	}
	if events[0].Message.Text != "hello there" {
		t.Errorf("content = %q", events[0].Message.Text)
	}
	if got := parseVibeLine("vibe 0.3.0 — mistral"); got != nil {
		t.Errorf("banner line produced events: %+v", got)
	}
	if got := parseVibeLine(`{"type":"usage","tokens":{}}`); got != nil {
		t.Errorf("usage line produced events before ISI-6204 pins the shape: %+v", got)
	}
}

func TestVibeSettleLine(t *testing.T) {
	for _, line := range []string{
		`{"type":"result"}`,
		`{"event":"done"}`,
		`{"type":"completed"}`,
	} {
		if !vibeSettleLine(line) {
			t.Errorf("SettleLine(%q) = false, want terminal marker", line)
		}
	}
	for _, line := range []string{
		`{"type":"assistant","text":"still working"}`,
		"plain text",
	} {
		if vibeSettleLine(line) {
			t.Errorf("SettleLine(%q) = true, want non-terminal", line)
		}
	}
}

func envGet(env []string, name string) string {
	for _, kv := range env {
		if strings.HasPrefix(kv, name+"=") {
			return strings.TrimPrefix(kv, name+"=")
		}
	}
	return ""
}

func envHas(env []string, name string) bool { return envGet(env, name) != "" }
