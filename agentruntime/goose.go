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
	"encoding/json"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
)

// Single-sourced goose constants (ISI-6128 P4.2, Phase A spike decision
// record @ 2b9a1ed): the pinned CLI revision and the default model + its
// context-window budget authority live here once, consumed by
// CLIVersion()/DefaultModel() and asserted by the unit tests. The spike
// pinned the static musl build of aaif-goose/goose v1.55.0 (the repo moved
// off block/goose on 2026-10-18); the surface is re-verified on every bump
// because upstream is mid-migration to client/server + ACP.
const (
	// gooseCLIVersion is the pinned goose revision the engine targets
	// (ADR-017 reproducibility; spike evidence — live probes of the binary).
	gooseCLIVersion = "1.55.0"
	// gooseDefaultModel is the runtime's default model id, the lane the
	// spike probed against the shared-LAN Ollama host. Agent.spec.model
	// overrides the id, not the window.
	gooseDefaultModel = "qwen3.5:35b"
	// gooseContextWindow is gooseDefaultModel's context-window budget
	// authority for the context Assembler (spec §6.2).
	gooseContextWindow = 131072
)

// The goose flavor env contract, locked by the spike against the pinned
// v1.55.0 binary (verified present; GOOSE_CONFIG_DIR verified ABSENT —
// config-dir isolation rides XDG_CONFIG_HOME, which the mesh adapter stamps
// on the standing process, not this engine).
const (
	gooseProviderEnv  = "GOOSE_PROVIDER"
	gooseModelEnv     = "GOOSE_MODEL"
	gooseModeEnv      = "GOOSE_MODE"
	gooseModeHeadless = "auto"
	gooseTelemetryOff = "GOOSE_TELEMETRY_OFF"
	gooseNoKeyringEnv = "GOOSE_DISABLE_KEYRING"
	// gooseOpenAIProvider is the provider lane a BYO OpenAI-compatible
	// ModelRoute rides: goose's openai provider consumes
	// OPENAI_BASE_URL/OPENAI_API_KEY (verified present in the v1.55.0
	// binary) — the same lane adapters #1/#2/#3 stamp for their BYO routes.
	gooseOpenAIProvider = "openai"
)

// goose is the goose coding-agent engine (ISI-6128 P4.2 Phase B; the mesh
// half is mesh.NewGooseAdapter, ISI-6197): the per-task one-shot
// `goose run -i - --output-format stream-json --no-session --no-profile`
// invocation locked by the Phase A spike decision. The prompt rides stdin
// (never argv — NFR-SEC1, the same invariant as the opencode run and
// vibe -p seams); a BYO OpenAI-compatible route rides goose's openai lane;
// the headless contract pins GOOSE_MODE=auto (no interactive approval), no
// telemetry and no keyring. Config-dir isolation is deliberately NOT
// re-derived here: the mesh adapter owns XDG_CONFIG_HOME on the standing
// process (mesh/goose.go) and the runner's append(os.Environ(), …)
// inherits it into the goose CLI — double-stamping from a task-relative
// WorkDir could split config.yaml/secrets.yaml/sessions.db away from the
// process's isolated dir.
type goose struct{}

func init() { Register(goose{}) }

func (goose) Type() string                     { return RuntimeTypeGoose }
func (goose) CLIVersion() string               { return gooseCLIVersion }
func (goose) CredentialShape() CredentialShape { return ShapeAPIKey }

// Capabilities is exactly the spike-proven surface: stream-json streaming,
// tool calls, a BYO OpenAI-compatible endpoint, and NO interactive approval
// (GOOSE_MODE=auto is headless). The unproven extras (docker, github,
// package install) stay unadvertised gaps until the tool-loop vocabulary is
// enumerated (spike risk note) — a gap is advertised, never discovered as a
// mid-Run failure.
func (goose) Capabilities() a2a.Capabilities {
	return a2a.Capabilities{
		Streaming:         true,
		ToolCalls:         true,
		InteractivePrompt: false,
		BYOModelEndpoint:  true, // openai lane (OPENAI_BASE_URL/OPENAI_API_KEY)
		// Declared EMPTY, deliberately: goose's file/patch production is
		// unproven in the spike capture (no tool loops ran), so the honest
		// posture is a declared-empty list — not nil (undeclared) and not a
		// guessed "file"/"patch" — until the tool-loop vocabulary is
		// enumerated and the artifact kinds are captured for real.
		ArtifactKinds: []string{},
	}
}

func (goose) DefaultModel() a2a.ModelInfo {
	return a2a.ModelInfo{ID: gooseDefaultModel, ContextWindow: gooseContextWindow}
}

func (r goose) Command(lc LaunchContext) (ExecSpec, error) {
	// The locked headless contract (spike decision): the sandbox has no
	// human to answer an approval prompt, no telemetry egress and no
	// keyring — goose falls back to its secrets.yaml inside the isolated
	// config dir the standing process provides.
	env := []string{
		gooseModeEnv + "=" + gooseModeHeadless,
		gooseTelemetryOff + "=1",
		gooseNoKeyringEnv + "=1",
	}
	// Pin the model the task drives: route model > Agent model > runtime
	// default (the shared precedence), re-pinned per task so an override
	// holds — the counterpart of the mesh adapter's standing-process pin.
	if model := resolveModel(r, lc); model != "" {
		env = append(env, gooseModelEnv+"="+model)
	}
	// Provider lane: a BYO OpenAI-compatible route selects goose's openai
	// lane and carries its own token+base URL (a zero-token route is the
	// conventional "ollama" placeholder lane); otherwise the per-user
	// credential maps onto OPENAI_API_KEY for whatever lane the host's
	// config.yaml pins. Emit exactly one key — emitting both would shadow
	// the route token (glibc getenv is first-wins), the codex/vibe rule.
	if route := modelRouteEnv(lc.ModelRoute); len(route) > 0 {
		env = append(env, gooseProviderEnv+"="+gooseOpenAIProvider)
		env = append(env, route...)
	} else if lc.Credential != "" {
		env = append(env, "OPENAI_API_KEY="+lc.Credential)
	}
	return ExecSpec{
		Path: "goose",
		Args: []string{
			"run", "-i", "-",
			"--output-format", "stream-json",
			"--no-session", "--no-profile",
		},
		Env:     env,
		WorkDir: lc.WorkDir,
		// The whole envelope rides stdin (system context + instruction,
		// NFR-SEC1): `goose run -i -` reads the prompt from stdin, keeping
		// it out of argv and the process table.
		Stdin:      EnvelopePrompt(lc),
		Parse:      parseGooseLine,
		SettleLine: gooseSettleLine,
	}, nil
}

// gooseContent is one content block of a stream-json message chunk
// (OpenAI-chat-completion shaped, per the v1.55.0 spike capture): the type
// discriminates the flavor — text blocks carry per-token text deltas,
// thinking blocks carry reasoning deltas. Tool block shapes are NOT yet
// enumerated (the spike probes ran no tool loops); unknown block types drop
// as bookkeeping until the vocabulary is captured, never guessed.
type gooseContent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
}

// parseGooseLine decodes one goose stream-json stdout line into typed
// Progress (ISI-6128 Phase A capture, v1.55.0):
//
//	{"type":"message","message":{"id":"chatcmpl-786","role":"assistant",
//	  "content":[{"type":"thinking","thinking":"The","signature":""}],…}}
//	{"type":"message","message":{"content":[{"type":"text","text":"SPI"}],…}}
//	{"type":"complete","total_tokens":10955,"input_tokens":10921,
//	  "output_tokens":34,"cache_read_input_tokens":0}
//
// The v1.55.0 banner preamble (the non-JSON session header goose prints
// before the first event) is skipped by the non-JSON guard — CLI chrome
// never reaches the wire. text deltas surface as message events at goose's
// native per-token granularity (the card advertises Streaming; consumers
// coalesce); thinking deltas and unknown block types drop. The terminal
// complete event maps onto EventUsage (input/output/cache-read, the same
// fields the codex turn.completed mapping carries) and doubles as the
// SettleLine marker. A JSON line of unknown type drops silently — parse
// failure must not kill a running task, and future upstream shapes degrade
// instead of erroring.
func parseGooseLine(line string) []Progress {
	var ev struct {
		Type    string `json:"type"`
		Message *struct {
			Role    string         `json:"role"`
			Content []gooseContent `json:"content"`
		} `json:"message"`
		TotalTokens      int `json:"total_tokens"`
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
		CacheReadTokens  int `json:"cache_read_input_tokens"`
		CacheWriteTokens int `json:"cache_write_input_tokens"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return nil // banner preamble / non-JSON chrome: skip-until-first-JSON
	}
	switch ev.Type {
	case "message":
		if ev.Message == nil {
			return nil
		}
		role := ev.Message.Role
		if role == "" {
			role = "assistant"
		}
		var out []Progress
		for _, b := range ev.Message.Content {
			if b.Type == "text" && b.Text != "" {
				out = append(out, Progress{
					Kind:    a2a.EventMessage,
					Message: &a2a.MessagePayload{Role: role, Text: b.Text, Trust: "untrusted"},
				})
			}
		}
		return out
	case "complete":
		return []Progress{{
			Kind: a2a.EventUsage,
			Usage: &a2a.UsagePayload{
				Input:     ev.InputTokens,
				Output:    ev.OutputTokens,
				CacheRead: ev.CacheReadTokens,
			},
		}}
	default: // future upstream shapes: bookkeeping
		return nil
	}
}

// gooseSettleLine reports whether one stdout line is goose's terminal
// event: a JSON object with type == "complete" — the natural SettleLine the
// spike locked (a pinned CLI can leave non-daemon handles holding its event
// loop open after the work finished, so the runner settles on the marker,
// not process exit). Pure; called from the runner's scanner goroutine.
func gooseSettleLine(line string) bool {
	var ev struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(line), &ev) == nil && ev.Type == "complete"
}
