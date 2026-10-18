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

// The vanilla-standalone regression test (ISI-6215): the goose flavor must
// resolve through the process-wide registry. This is exactly the lookup
// cmd/agentruntime (KSQUAD_RUNTIME_TYPE) and cmd/mesh-gateway
// (MESH_RUNTIME_TYPE) perform — and the lookup that exit(1)'d both before
// the engine existed, when only codex and vibe were registered, leaving
// mesh.NewGooseAdapter's standing process unable to build its engine on a
// host-free deployment.
func TestGooseRegistryEntry(t *testing.T) {
	rt, err := Get(RuntimeTypeGoose)
	if err != nil {
		t.Fatalf("Get(goose): %v", err)
	}
	if got := rt.CLIVersion(); got != "1.55.0" {
		t.Errorf("CLIVersion = %q, want 1.55.0 (spike binary pin)", got)
	}
	if got := rt.Type(); got != RuntimeTypeGoose {
		t.Errorf("Type = %q, want %q", got, RuntimeTypeGoose)
	}
	if rt.CredentialShape() != ShapeAPIKey {
		t.Errorf("CredentialShape = %q, want api-key", rt.CredentialShape())
	}
	dm := rt.DefaultModel()
	if dm.ID != "qwen3.5:35b" || dm.ContextWindow != 131072 {
		t.Errorf("DefaultModel = %+v, want qwen3.5:35b/131072", dm)
	}
	caps := rt.Capabilities()
	if !caps.Streaming || !caps.ToolCalls || caps.InteractivePrompt {
		t.Errorf("Capabilities = %+v, want streaming+toolcalls, headless (no interactive)", caps)
	}
	if !caps.BYOModelEndpoint {
		t.Errorf("Capabilities.BYOModelEndpoint = false, want true (openai lane)")
	}
	// Honest-gap posture (spike risk note): the unproven extras stay
	// unadvertised until the tool-loop vocabulary is enumerated.
	if caps.Docker || caps.GitHub || caps.PackageInstall {
		t.Errorf("Capabilities = %+v, want the unproven extras unadvertised", caps)
	}
}

func TestGooseCommandShape(t *testing.T) {
	rt := goose{}
	spec, err := rt.Command(LaunchContext{
		Envelope: a2a.Envelope{SystemContext: "sys", Input: "say SPIKEOK"},
		WorkDir:  "/wt/42",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if spec.Path != "goose" {
		t.Fatalf("Path = %q, want goose", spec.Path)
	}
	wantArgs := strings.Join([]string{
		"run", "-i", "-",
		"--output-format", "stream-json",
		"--no-session", "--no-profile",
	}, " ")
	if got := strings.Join(spec.Args, " "); got != wantArgs {
		t.Errorf("Args = %q, want %q", got, wantArgs)
	}
	if spec.Stdin != "sys\n\nsay SPIKEOK" {
		t.Errorf("Stdin = %q, want the reassembled envelope prompt", spec.Stdin)
	}
	// The locked headless contract is stamped on every launch.
	for _, want := range []string{
		"GOOSE_MODE=auto",
		"GOOSE_TELEMETRY_OFF=1",
		"GOOSE_DISABLE_KEYRING=1",
	} {
		if !envHas(spec.Env, strings.SplitN(want, "=", 2)[0]) || envGet(spec.Env, strings.SplitN(want, "=", 2)[0]) != strings.SplitN(want, "=", 2)[1] {
			t.Errorf("env %v missing %s (headless lock)", spec.Env, want)
		}
	}
	// The default-model pin: no route and no task model → runtime default.
	if got := envGet(spec.Env, "GOOSE_MODEL"); got != "qwen3.5:35b" {
		t.Errorf("GOOSE_MODEL = %q, want the runtime default pin", got)
	}
	// With neither a route nor a credential the engine guesses no lane.
	if envHas(spec.Env, "GOOSE_PROVIDER") {
		t.Errorf("GOOSE_PROVIDER set with no route; want no lane guess")
	}
	if envHas(spec.Env, "OPENAI_API_KEY") {
		t.Errorf("OPENAI_API_KEY set with no credential and no route; want empty lane")
	}
}

func TestGooseCommandModelRoute(t *testing.T) {
	rt := goose{}
	spec, err := rt.Command(LaunchContext{
		Envelope:   a2a.Envelope{Input: "work"},
		ModelRoute: a2a.ModelRoute{Endpoint: "http://ollama:11434/v1", Model: "qwen3.5:122B"},
		WorkDir:    "/wt/7",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got := envGet(spec.Env, "GOOSE_PROVIDER"); got != "openai" {
		t.Errorf("GOOSE_PROVIDER = %q, want openai (BYO lane)", got)
	}
	if got := envGet(spec.Env, "OPENAI_BASE_URL"); got != "http://ollama:11434/v1" {
		t.Errorf("OPENAI_BASE_URL = %q, want the route endpoint", got)
	}
	// The zero-credential Ollama lane rides the conventional placeholder.
	if got := envGet(spec.Env, "OPENAI_API_KEY"); got != "ollama" {
		t.Errorf("OPENAI_API_KEY = %q, want the ollama placeholder", got)
	}
	if got := envGet(spec.Env, "GOOSE_MODEL"); got != "qwen3.5:122B" {
		t.Errorf("GOOSE_MODEL = %q, want the route model (route wins)", got)
	}
}

func TestGooseCommandCredential(t *testing.T) {
	rt := goose{}
	spec, err := rt.Command(LaunchContext{
		Envelope:   a2a.Envelope{Input: "work"},
		Credential: "sk-test-goose",
		WorkDir:    "/wt/9",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got := envGet(spec.Env, "OPENAI_API_KEY"); got != "sk-test-goose" {
		t.Errorf("OPENAI_API_KEY = %q, want the per-user credential", got)
	}
	if envHas(spec.Env, "OPENAI_BASE_URL") {
		t.Errorf("OPENAI_BASE_URL set without a route; want vendor wire")
	}
	if envHas(spec.Env, "GOOSE_PROVIDER") {
		t.Errorf("GOOSE_PROVIDER set without a route; want the host config's lane")
	}
}

// The stream lines below are lifted verbatim from the v1.55.0 spike capture
// (~/.paperclip-spike/isi-6128/run_stream_stdout.txt, ISI-6128 Phase A) —
// banner preamble, thinking deltas, per-token text deltas and the terminal
// complete event with its usage block.
func TestParseGooseLineBannerAndDeltas(t *testing.T) {
	// Banner preamble: non-JSON chrome before the first event.
	for _, line := range []string{
		"",
		"    __( O)>  ● new session · ollama qwen3.5:35b",
		`   \____)    20261018_1 · /home/hrexed/.paperclip-spike/isi-6128`,
		"     L L     goose is ready",
	} {
		if got := parseGooseLine(line); got != nil {
			t.Errorf("parseGooseLine(banner %q) = %v, want nil (skip-until-JSON)", line, got)
		}
	}
	// A thinking delta: bookkeeping, dropped (the codex reasoning posture).
	thinking := `{"type":"message","message":{"id":"chatcmpl-786","role":"assistant","created":1792329755,"content":[{"type":"thinking","thinking":"The","signature":""}],"metadata":{"userVisible":true,"agentVisible":true,"inference":{"provider":"ollama","requestedModel":"qwen3.5:35b"},"operations":{"llm":{"advertised_tools":["load_skill"]}}}}}`
	if got := parseGooseLine(thinking); got != nil {
		t.Errorf("parseGooseLine(thinking) = %v, want nil (bookkeeping)", got)
	}
	// A text delta: one message event at goose's native granularity.
	text := `{"type":"message","message":{"id":"chatcmpl-786","role":"assistant","created":1792329756,"content":[{"type":"text","text":"SPI"}],"metadata":{"userVisible":true,"agentVisible":true,"inference":{"provider":"ollama","requestedModel":"qwen3.5:35b"},"operations":{"llm":{"advertised_tools":["load_skill"]}}}}}`
	got := parseGooseLine(text)
	if len(got) != 1 || got[0].Kind != a2a.EventMessage || got[0].Message == nil {
		t.Fatalf("parseGooseLine(text) = %+v, want one message event", got)
	}
	if got[0].Message.Role != "assistant" || got[0].Message.Text != "SPI" {
		t.Errorf("message = %+v, want assistant/SPI", got[0].Message)
	}
	// Unknown future event type: degrades silently, never errors.
	if got := parseGooseLine(`{"type":"session.saved","path":"/x"}`); got != nil {
		t.Errorf("parseGooseLine(unknown type) = %v, want nil", got)
	}
}

func TestParseGooseLineComplete(t *testing.T) {
	line := `{"type":"complete","total_tokens":10955,"input_tokens":10921,"output_tokens":34,"cache_read_input_tokens":0,"cache_write_input_tokens":0}`
	got := parseGooseLine(line)
	if len(got) != 1 || got[0].Kind != a2a.EventUsage || got[0].Usage == nil {
		t.Fatalf("parseGooseLine(complete) = %+v, want one usage event", got)
	}
	u := got[0].Usage
	if u.Input != 10921 || u.Output != 34 || u.CacheRead != 0 {
		t.Errorf("usage = %+v, want input=10921 output=34 cacheRead=0", u)
	}
}

func TestGooseSettleLine(t *testing.T) {
	complete := `{"type":"complete","total_tokens":10955,"input_tokens":10921,"output_tokens":34}`
	if !gooseSettleLine(complete) {
		t.Errorf("gooseSettleLine(complete) = false, want true (terminal marker)")
	}
	message := `{"type":"message","message":{"content":[{"type":"text","text":"OK"}]}}`
	if gooseSettleLine(message) {
		t.Errorf("gooseSettleLine(message) = true, want false")
	}
	if gooseSettleLine("     L L     goose is ready") {
		t.Errorf("gooseSettleLine(banner) = true, want false")
	}
}
