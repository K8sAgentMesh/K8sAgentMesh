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

// The P4.4 promotion regression test (ISI-6130): the claude-code flavor must
// resolve through the process-wide registry — the lookup cmd/agentruntime and
// cmd/mesh-supervisor perform, and the lookup that failed closed through
// Phase 2 before the engine existed.
func TestClaudeCodeRegistryEntry(t *testing.T) {
	rt, err := Get(RuntimeTypeClaudeCode)
	if err != nil {
		t.Fatalf("Get(claude-code): %v", err)
	}
	if got := rt.Type(); got != RuntimeTypeClaudeCode {
		t.Errorf("Type = %q, want %q", got, RuntimeTypeClaudeCode)
	}
	if got := rt.CLIVersion(); got != claudeCodeCLIVersion {
		t.Errorf("CLIVersion = %q, want %q (workspace-verified pin)", got, claudeCodeCLIVersion)
	}
	if rt.CredentialShape() != ShapeOAuthToken {
		t.Errorf("CredentialShape = %q, want oauth-token (story 7.2)", rt.CredentialShape())
	}
	dm := rt.DefaultModel()
	if dm.ID != claudeCodeDefaultModel || dm.ContextWindow != claudeCodeContextWindow {
		t.Errorf("DefaultModel = %+v, want %s/%d", dm, claudeCodeDefaultModel, claudeCodeContextWindow)
	}
	caps := rt.Capabilities()
	if !caps.Streaming || !caps.ToolCalls {
		t.Errorf("Capabilities = %+v, want streaming+toolcalls", caps)
	}
	if caps.InteractivePrompt {
		t.Error("Capabilities.InteractivePrompt = true, want false (headless -p)")
	}
	// The honest BYO gap: Claude Code speaks the Anthropic wire; the seam's
	// OpenAI-compatible ModelRoute cannot be honored.
	if caps.BYOModelEndpoint {
		t.Error("Capabilities.BYOModelEndpoint = true, want false (Anthropic wire — advertised gap)")
	}
}

func TestClaudeCodeCommandShape(t *testing.T) {
	rt := claudeCode{}
	spec, err := rt.Command(LaunchContext{
		Envelope: a2a.Envelope{SystemContext: "sys", Input: "say hi"},
		WorkDir:  "/wt/42",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if spec.Path != "claude" {
		t.Fatalf("Path = %q, want claude", spec.Path)
	}
	wantArgs := strings.Join([]string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
		"--model", claudeCodeDefaultModel,
	}, " ")
	if got := strings.Join(spec.Args, " "); got != wantArgs {
		t.Errorf("Args = %q, want %q", got, wantArgs)
	}
	if spec.Stdin != "sys\n\nsay hi" {
		t.Errorf("Stdin = %q, want the reassembled envelope prompt", spec.Stdin)
	}
	if spec.WorkDir != "/wt/42" {
		t.Errorf("WorkDir = %q, want /wt/42", spec.WorkDir)
	}
	// The sandbox-hygiene contract is stamped on every launch.
	for _, want := range []string{"DISABLE_TELEMETRY=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"} {
		if !envHas(spec.Env, strings.SplitN(want, "=", 2)[0]) {
			t.Errorf("env %v missing %s (hygiene lock)", spec.Env, want)
		}
	}
	// No credential and no route: no token env, no route guess.
	if envHas(spec.Env, claudeCodeOAuthEnv) {
		t.Errorf("env %v sets %s with no credential", spec.Env, claudeCodeOAuthEnv)
	}
}

func TestClaudeCodeCommandModelOverride(t *testing.T) {
	rt := claudeCode{}
	spec, err := rt.Command(LaunchContext{
		Envelope: a2a.Envelope{Input: "work"},
		Model:    "claude-opus-4",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if !hasArg(spec.Args, "claude-opus-4") {
		t.Errorf("Args = %v, want the model override via --model", spec.Args)
	}
}

func TestClaudeCodeCommandCredential(t *testing.T) {
	rt := claudeCode{}
	spec, err := rt.Command(LaunchContext{
		Envelope:   a2a.Envelope{Input: "work"},
		Credential: "oauth-seat-token",
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got := envGet(spec.Env, claudeCodeOAuthEnv); got != "oauth-seat-token" {
		t.Errorf("%s = %q, want the per-user OAuth credential", claudeCodeOAuthEnv, got)
	}
	for _, a := range spec.Args {
		if strings.Contains(a, "oauth-seat-token") {
			t.Errorf("credential leaked into argv %q", a)
		}
	}
}

// A BYO route must NOT be mapped onto any endpoint env: the route is the
// OpenAI-compatible wire and claude-code speaks Anthropic — the advertised
// BYOModelEndpoint gap (the same fail-closed posture the baseURLEnv table
// enforces host-side).
func TestClaudeCodeCommandIgnoresOpenAICompatibleRoute(t *testing.T) {
	rt := claudeCode{}
	spec, err := rt.Command(LaunchContext{
		Envelope:   a2a.Envelope{Input: "work"},
		Credential: "seat",
		ModelRoute: a2a.ModelRoute{Endpoint: "http://ollama:11434/v1", Model: "qwen3", Token: "rt"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	for _, unwanted := range []string{"OPENAI_BASE_URL", "OPENAI_API_KEY", "ANTHROPIC_BASE_URL", claudeCodeModelEnv} {
		if envHas(spec.Env, unwanted) {
			t.Errorf("env %v sets %s for an OpenAI-compatible route (want no mapping)", spec.Env, unwanted)
		}
	}
}

// The stream lines below follow the Claude Code 2.1.239 headless stream-json
// wire: system init, an assistant turn with text + a Bash tool_use, the
// settled tool_result on the user message, and the terminal result event.
func TestParseClaudeCodeLineAssistant(t *testing.T) {
	names := map[string]string{}
	init := `{"type":"system","subtype":"init","cwd":"/w","session_id":"s1","tools":["Bash"],"model":"claude-sonnet-4"}`
	if got := parseClaudeCodeLine(init, names); got != nil {
		t.Errorf("parseClaudeCodeLine(init) = %v, want nil (bookkeeping)", got)
	}
	assistant := `{"type":"assistant","message":{"id":"msg_1","role":"assistant","model":"claude-sonnet-4","content":[{"type":"text","text":"Checking."},{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"git status","description":"repo state"}}]},"session_id":"s1"}`
	got := parseClaudeCodeLine(assistant, names)
	if len(got) != 2 {
		t.Fatalf("parseClaudeCodeLine(assistant) = %+v, want one message + one tool-start", got)
	}
	if got[0].Kind != a2a.EventMessage || got[0].Message == nil ||
		got[0].Message.Role != "assistant" || got[0].Message.Text != "Checking." {
		t.Errorf("message = %+v, want assistant/Checking.", got[0].Message)
	}
	tool := got[1].Tool
	if got[1].Kind != a2a.EventTool || tool == nil || tool.Name != "Bash" || tool.Phase != "start" {
		t.Fatalf("tool = %+v, want Bash/start", got[1])
	}
	// ISI-4720: the recognized executable head is extracted from the command
	// argument, in-process, before args are hashed.
	if tool.Command != "git" {
		t.Errorf("tool.Command = %q, want git (shell-head enrichment)", tool.Command)
	}
	if got[1].ToolArgs == "" || !strings.Contains(got[1].ToolArgs, "git status") {
		t.Errorf("ToolArgs = %q, want the raw input JSON", got[1].ToolArgs)
	}
	if names["toolu_1"] != "Bash" {
		t.Errorf("id correlation = %v, want toolu_1→Bash", names)
	}
}

func TestParseClaudeCodeLineToolResultSettles(t *testing.T) {
	names := map[string]string{"toolu_1": "Bash"}
	user := `{"type":"user","message":{"id":"msg_2","role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"On branch main\nnothing to commit","is_error":false}]},"session_id":"s1"}`
	got := parseClaudeCodeLine(user, names)
	if len(got) != 1 || got[0].Kind != a2a.EventTool || got[0].Tool == nil {
		t.Fatalf("parseClaudeCodeLine(tool_result) = %+v, want one tool-result", got)
	}
	tool := got[0].Tool
	if tool.Name != "Bash" || tool.Phase != "result" || tool.OK == nil || !*tool.OK {
		t.Errorf("tool = %+v, want Bash/result/ok", tool)
	}
	// An error result surfaces the bounded failure reason (ISI-5211 posture).
	errUser := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"text","text":"fatal: not a git repository"}],"is_error":true}]}}`
	got = parseClaudeCodeLine(errUser, names)
	if len(got) != 1 || got[0].Tool == nil || got[0].Tool.OK == nil || *got[0].Tool.OK {
		t.Fatalf("parseClaudeCodeLine(error result) = %+v, want not-ok", got)
	}
	if got[0].Tool.Summary == "" {
		t.Error("error result carries no failure summary")
	}
	// A result for a call this run never saw stays bookkeeping.
	unknown := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_x","content":"…"}]}}`
	if g := parseClaudeCodeLine(unknown, names); g != nil {
		t.Errorf("parseClaudeCodeLine(unknown id) = %v, want nil", g)
	}
}

func TestParseClaudeCodeLineResult(t *testing.T) {
	line := `{"type":"result","subtype":"success","is_error":false,"duration_ms":5192,"num_turns":4,"result":"All tests pass","session_id":"s1","total_cost_usd":0.01341,"usage":{"input_tokens":378,"cache_creation_input_tokens":1712,"cache_read_input_tokens":0,"output_tokens":52}}`
	got := parseClaudeCodeLine(line, map[string]string{})
	if len(got) != 1 || got[0].Kind != a2a.EventUsage || got[0].Usage == nil {
		t.Fatalf("parseClaudeCodeLine(result) = %+v, want one usage event", got)
	}
	u := got[0].Usage
	if u.Input != 378 || u.Output != 52 || u.CacheRead != 0 || u.CacheWrite != 1712 {
		t.Errorf("usage tokens = %+v, want input=378 output=52 cacheWrite=1712", u)
	}
	if u.CostUSD != 0.01341 || u.DurationMS != 5192 {
		t.Errorf("usage cost/duration = %+v, want 0.01341/5192ms", u)
	}
	// An errored result also surfaces its reason as a message.
	errLine := `{"type":"result","subtype":"error_max_turns","is_error":true,"duration_ms":90000,"result":"The agent ran out of turns","usage":{"input_tokens":900,"output_tokens":800}}`
	got = parseClaudeCodeLine(errLine, map[string]string{})
	if len(got) != 2 || got[0].Kind != a2a.EventMessage || got[1].Kind != a2a.EventUsage {
		t.Fatalf("parseClaudeCodeLine(error result) = %+v, want message+usage", got)
	}
	if !strings.Contains(got[0].Message.Text, "out of turns") {
		t.Errorf("error message = %q, want the result reason", got[0].Message.Text)
	}
}

func TestParseClaudeCodeLineNonJSONDegrades(t *testing.T) {
	got := parseClaudeCodeLine("Authentication required. Run /login", map[string]string{})
	if len(got) != 1 || got[0].Kind != a2a.EventMessage || got[0].Message.Text != "Authentication required. Run /login" {
		t.Fatalf("parseClaudeCodeLine(non-JSON) = %+v, want one degraded message", got)
	}
}

func TestClaudeCodeSettleLine(t *testing.T) {
	result := `{"type":"result","subtype":"success","is_error":false,"result":"done"}`
	if !claudeCodeSettleLine(result) {
		t.Error("claudeCodeSettleLine(result) = false, want true (terminal marker)")
	}
	assistant := `{"type":"assistant","message":{"content":[{"type":"text","text":"OK"}]}}`
	if claudeCodeSettleLine(assistant) {
		t.Error("claudeCodeSettleLine(assistant) = true, want false")
	}
	if claudeCodeSettleLine("not json") {
		t.Error("claudeCodeSettleLine(non-JSON) = true, want false")
	}
}

// The per-run parser correlates ids across lines: the tool_result on the user
// message settles the span the assistant tool_use opened, by name.
func TestClaudeCodeParserCorrelatesAcrossLines(t *testing.T) {
	parse := newClaudeCodeParser()
	start := parse(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_9","name":"Read","input":{"file_path":"/w/main.go"}}]}}`)
	if len(start) != 1 || start[0].Tool.Name != "Read" || start[0].Tool.Phase != "start" {
		t.Fatalf("start = %+v, want Read/start", start)
	}
	end := parse(`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_9","content":"package main","is_error":false}]}}`)
	if len(end) != 1 || end[0].Tool.Name != "Read" || end[0].Tool.Phase != "result" {
		t.Fatalf("end = %+v, want Read/result (correlated)", end)
	}
}
