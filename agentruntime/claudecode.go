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
	"strings"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
)

// Single-sourced Claude Code constants (P4.4, ISI-6130): the pinned CLI
// revision and the default model + its context-window budget authority live
// here once, consumed by CLIVersion()/DefaultModel() and asserted by the unit
// tests. The pin is the workspace-verified `claude --version` (2.1.239) —
// re-verify on every bump (ADR-017).
const (
	// claudeCodeCLIVersion is the pinned Claude Code CLI revision the engine
	// targets (ADR-017 reproducibility).
	claudeCodeCLIVersion = "2.1.239"
	// claudeCodeDefaultModel is the runtime's default model id — the same
	// Claude-family default the opencode/openclaw engines pin. Agent.spec.model
	// overrides the id (--model), not the window.
	claudeCodeDefaultModel = "claude-sonnet-4"
	// claudeCodeContextWindow is claudeCodeDefaultModel's context-window budget
	// authority for the context Assembler (spec §6.2).
	claudeCodeContextWindow = 200000
)

// The claude-code flavor env contract. The OAuth credential rides
// CLAUDE_CODE_OAUTH_TOKEN (story 7.2, the subscription-seat human-seat class —
// credinject's (claude-code, human-seat) row maps the generic Secret onto the
// same name); DISABLE_TELEMETRY and CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC
// are the sandbox-hygiene knobs (the goose telemetry-off / opencode
// autoupdate+filewatcher analogues): no telemetry emission, and no
// non-essential network (update checks, error reporting) — the linger sources
// that can hold a finished one-shot's event loop open past stdout EOF.
const (
	claudeCodeOAuthEnv        = "CLAUDE_CODE_OAUTH_TOKEN"
	claudeCodeTelemetryOffEnv = "DISABLE_TELEMETRY"
	claudeCodeNoExtraTraffic  = "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"
	// claudeCodeModelEnv pins the standing process's default model
	// (ANTHROPIC_MODEL, the CLI's documented model-select env); this engine
	// re-pins per task via --model so an override holds.
	claudeCodeModelEnv = "ANTHROPIC_MODEL"
)

// claudeCode is the Claude Code coding-agent engine (P4.4, ISI-6130): the
// per-task one-shot `claude -p --output-format stream-json --verbose`
// invocation. --verbose is load-bearing: without it the CLI's stream-json
// mode emits only the init + result events, not the assistant/user event
// stream — the full wire (assistant text/tool_use blocks, user tool_result
// blocks, the terminal result event) is the --verbose shape. --dangerously-
// skip-permissions is the headless contract (the goose GOOSE_MODE=auto /
// vibe --auto-approve analogue): the sandbox has no human to answer a
// permission prompt. The prompt rides stdin (never argv — NFR-SEC1); the
// per-user OAuth credential maps onto CLAUDE_CODE_OAUTH_TOKEN
// (ShapeOAuthToken, story 7.2).
type claudeCode struct{}

func init() { Register(claudeCode{}) }

func (claudeCode) Type() string                     { return RuntimeTypeClaudeCode }
func (claudeCode) CLIVersion() string               { return claudeCodeCLIVersion }
func (claudeCode) CredentialShape() CredentialShape { return ShapeOAuthToken }

// Capabilities is the honest P4.4 surface: stream-json streaming, tool calls,
// file/patch artifacts (the Write/Edit tools), and NO interactive approval
// (headless -p). BYOModelEndpoint is an advertised GAP: Claude Code speaks
// the Anthropic wire, and the seam's ModelRoute is the OpenAI-compatible
// wire — an OpenAI-compatible endpoint cannot be honored without a
// translating proxy, so the engine maps no route env and the card says so
// rather than silently sending runs to a provider default. The unproven
// extras (docker, github, package install) stay unadvertised until the
// tool-loop vocabulary is captured live (the goose posture).
func (claudeCode) Capabilities() a2a.Capabilities {
	return a2a.Capabilities{
		Streaming:         true,
		ToolCalls:         true,
		InteractivePrompt: false, // headless -p, by design
		BYOModelEndpoint:  false, // Anthropic wire only — advertised gap
		ArtifactKinds:     []string{"file", "patch"},
	}
}

func (claudeCode) DefaultModel() a2a.ModelInfo {
	return a2a.ModelInfo{ID: claudeCodeDefaultModel, ContextWindow: claudeCodeContextWindow}
}

func (r claudeCode) Command(lc LaunchContext) (ExecSpec, error) {
	// The sandbox-hygiene contract: no telemetry, no non-essential network —
	// the linger sources that would hold a finished one-shot open (ISI-4224
	// posture: suppress at the source, SettleLine is the backstop).
	env := []string{
		claudeCodeTelemetryOffEnv + "=1",
		claudeCodeNoExtraTraffic + "=1",
	}
	// The per-user OAuth seat (story 7.2) — never logged, never argv.
	if lc.Credential != "" {
		env = append(env, claudeCodeOAuthEnv+"="+lc.Credential)
	}
	// A BYO ModelRoute is deliberately NOT mapped: the route is the
	// OpenAI-compatible wire and Claude Code speaks Anthropic (the advertised
	// BYOModelEndpoint gap). Emitting OPENAI_* here would be ignored; emitting
	// ANTHROPIC_BASE_URL would point an Anthropic client at an
	// OpenAI-compatible endpoint and fail mid-Run — fail closed by saying
	// nothing and letting the card's gap advertise it.
	return ExecSpec{
		Path: "claude",
		Args: []string{
			"-p",
			"--output-format", "stream-json",
			"--verbose",
			"--dangerously-skip-permissions",
			"--model", resolveModel(r, lc),
		},
		Env:     env,
		WorkDir: lc.WorkDir,
		// The whole envelope rides stdin (system context + instruction,
		// NFR-SEC1): `claude -p` reads its prompt from stdin when no
		// positional prompt is given, keeping it out of argv and the process
		// table — the same seam as the opencode run and goose run -i -.
		Stdin:      EnvelopePrompt(lc),
		Parse:      newClaudeCodeParser(),
		SettleLine: claudeCodeSettleLine,
	}, nil
}

// claudeCodeContent is one content block of a stream-json assistant/user
// message (Anthropic-message shaped): text blocks carry assistant text,
// tool_use blocks carry a tool call (id + name + raw input JSON), tool_result
// blocks (on user messages) carry the settled outcome of the call the id
// names. Unknown block types (thinking, image, …) drop as bookkeeping.
type claudeCodeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
	// tool_use (assistant blocks).
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
	// tool_result (user blocks).
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// newClaudeCodeParser returns a per-run stateful stdout parser: it wraps the
// stateless decode and correlates tool_use ids to names so a user-message
// tool_result (which carries only the id) settles as a phase="result"
// EventTool named for the call that produced it — one span per call settles,
// the opencode/codex posture. State is per-run because Command builds a fresh
// ExecSpec (hence a fresh parser) for each Run, so no cross-run leakage is
// possible.
func newClaudeCodeParser() func(line string) []Progress {
	names := map[string]string{} // tool_use id → tool name
	return func(line string) []Progress {
		return parseClaudeCodeLine(line, names)
	}
}

// parseClaudeCodeLine decodes one `--output-format stream-json --verbose`
// stdout line into typed Progress. The wire (Claude Code 2.1.239 headless):
//
//	{"type":"system","subtype":"init",…}                     (bookkeeping)
//	{"type":"assistant","message":{"role":"assistant","content":[
//	  {"type":"text","text":"…"},
//	  {"type":"tool_use","id":"toolu_…","name":"Bash",
//	   "input":{"command":"git status"}}]}}
//	{"type":"user","message":{"role":"user","content":[
//	  {"type":"tool_result","tool_use_id":"toolu_…",
//	   "content":"On branch main…","is_error":false}]}}
//	{"type":"result","subtype":"success","is_error":false,
//	  "duration_ms":5192,"result":"done","total_cost_usd":0.013,
//	  "usage":{"input_tokens":378,"output_tokens":52,
//	  "cache_read_input_tokens":0,"cache_creation_input_tokens":0}}
//
// assistant text blocks surface as message events; tool_use blocks record the
// id→name pair and emit a phase="start" tool event; tool_result blocks settle
// the call (phase="result", tri-state OK from is_error, the bounded failure
// reason on error). The terminal result event maps onto EventUsage
// (input/output/cache-read/cache-write + total_cost_usd + duration_ms) and an
// error message when is_error. system/unknown types drop; a non-JSON line
// degrades to a message event (the codex posture) — parse failure must not
// kill a running task.
func parseClaudeCodeLine(line string, names map[string]string) []Progress {
	var ev struct {
		Type    string `json:"type"`
		Message *struct {
			Role    string              `json:"role"`
			Content []claudeCodeContent `json:"content"`
		} `json:"message"`
		// result event fields.
		Subtype      string  `json:"subtype"`
		IsError      bool    `json:"is_error"`
		Result       string  `json:"result"`
		DurationMS   int64   `json:"duration_ms"`
		TotalCostUSD float64 `json:"total_cost_usd"`
		Usage        *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return []Progress{{
			Kind:    a2a.EventMessage,
			Message: &a2a.MessagePayload{Role: "agent", Text: line, Trust: "untrusted"},
		}}
	}
	switch ev.Type {
	case "assistant":
		if ev.Message == nil {
			return nil
		}
		var out []Progress
		for _, b := range ev.Message.Content {
			switch b.Type {
			case "text":
				if b.Text == "" {
					continue
				}
				out = append(out, Progress{
					Kind:    a2a.EventMessage,
					Message: &a2a.MessagePayload{Role: "assistant", Text: b.Text, Trust: "untrusted"},
				})
			case "tool_use":
				if b.Name == "" {
					continue
				}
				if b.ID != "" {
					names[b.ID] = b.Name
				}
				tool := &a2a.ToolPayload{Name: b.Name, Phase: "start"}
				var args string
				if len(b.Input) > 0 {
					args = string(b.Input)
				}
				// ISI-4720 reuse: a Bash tool call's real work is the
				// "command" argument — extract the recognized executable head
				// in-process, before args are hashed, so the telemetry spine
				// categorizes the call by what it RAN (never the whole line).
				if isClaudeCodeShellTool(b.Name) {
					if head := a2a.ShellCommandHead(claudeCodeCommandArg(b.Input)); head != "" {
						tool.Command = head
					}
				}
				out = append(out, Progress{Kind: a2a.EventTool, Tool: tool, ToolArgs: args})
			}
		}
		return out
	case "user":
		if ev.Message == nil {
			return nil
		}
		var out []Progress
		for _, b := range ev.Message.Content {
			if b.Type != "tool_result" {
				continue
			}
			name := names[b.ToolUseID]
			if name == "" {
				continue // a result for a call this run never saw: bookkeeping
			}
			ok := !b.IsError
			tool := &a2a.ToolPayload{Name: name, Phase: "result", OK: &ok}
			if b.IsError {
				if reason := claudeCodeResultText(b.Content); reason != "" {
					tool.Summary = a2a.ToolErrorSummary(reason)
				}
			}
			out = append(out, Progress{Kind: a2a.EventTool, Tool: tool})
		}
		return out
	case "result":
		var out []Progress
		if ev.IsError {
			msg := ev.Result
			if msg == "" {
				msg = "claude code error"
				if ev.Subtype != "" {
					msg = "claude code error: " + ev.Subtype
				}
			}
			out = append(out, Progress{
				Kind:    a2a.EventMessage,
				Message: &a2a.MessagePayload{Role: "agent", Text: msg, Trust: "untrusted"},
			})
		}
		if u := ev.Usage; u != nil {
			out = append(out, Progress{
				Kind: a2a.EventUsage,
				Usage: &a2a.UsagePayload{
					Input:      u.InputTokens,
					Output:     u.OutputTokens,
					CacheRead:  u.CacheReadInputTokens,
					CacheWrite: u.CacheCreationInputTokens,
					CostUSD:    ev.TotalCostUSD,
					DurationMS: ev.DurationMS,
				},
			})
		}
		return out
	default: // system/init and future shapes: bookkeeping
		return nil
	}
}

// claudeCodeSettleLine reports whether one stdout line is the stream-json
// terminal event: a JSON object with type == "result" — the CLI's "the
// session's work is done" marker (the goose complete / opencode step_finish
// analogue). One-shot -p normally exits on its own; the settle quiet window
// is the backstop for the non-daemon linger sources the hygiene env does not
// cover (ISI-4224). Pure; called from the runner's scanner goroutine.
func claudeCodeSettleLine(line string) bool {
	var ev struct {
		Type string `json:"type"`
	}
	return json.Unmarshal([]byte(line), &ev) == nil && ev.Type == "result"
}

// isClaudeCodeShellTool reports whether a Claude Code tool name is the
// shell-family tool whose real work is an arbitrary command line carried as
// the "command" input argument (ISI-4720).
func isClaudeCodeShellTool(tool string) bool {
	return strings.EqualFold(tool, "Bash")
}

// claudeCodeCommandArg pulls the raw "command" string out of a Bash tool's
// input JSON. It tolerates any other shape — a missing or malformed command
// yields "" and the call stays a plain shell span (never fails the parse).
func claudeCodeCommandArg(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var in struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return ""
	}
	return in.Command
}

// claudeCodeResultText reads a tool_result content block, tolerating both the
// plain-string form ("On branch main…") and the block-array form
// ([{"type":"text","text":"…"}]). An unreadable shape yields "".
func claudeCodeResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []claudeCodeContent
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var sb strings.Builder
		for _, b := range blocks {
			if b.Text != "" {
				if sb.Len() > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(b.Text)
			}
		}
		return sb.String()
	}
	return ""
}
