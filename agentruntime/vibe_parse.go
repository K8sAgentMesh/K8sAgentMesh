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

// vibeStreamEvent is one NDJSON record of `vibe -p --output streaming`.
// The engine decodes the envelope fields it understands and ignores the
// rest (forward-compatible: unknown types yield no Progress rather than an
// error — the runner's settle path, not the parser, owns completion).
//
// NOTE(ISI-6203): the marker vocabulary below covers the documented
// message/assistant/tool/result shapes of the py-v0.3.0 stream. The kind
// validation pass (ISI-6204, vanilla kind install) pins the exact emitted
// markers against the pinned CLI and extends this map if the live stream
// differs — the parse is deliberately permissive so an unrecognized marker
// degrades to silence, never to a failed run.
type vibeStreamEvent struct {
	Type    string          `json:"type"`
	Event   string          `json:"event"`
	Role    string          `json:"role"`
	Content string          `json:"content"`
	Text    string          `json:"text"`
	Message json.RawMessage `json:"message"`
	Tool    string          `json:"tool"`
	Error   string          `json:"error"`
}

func (e vibeStreamEvent) kind() string {
	if e.Type != "" {
		return e.Type
	}
	return e.Event
}

func (e vibeStreamEvent) body() string {
	switch {
	case e.Text != "":
		return e.Text
	case e.Content != "":
		return e.Content
	}
	return ""
}

// parseVibeLine maps one streaming stdout line onto zero or more Progress
// events. Non-JSON lines yield nothing (CLI banner/noise); the runner's
// line→message fallback is disabled for vibe because the NDJSON stream is
// authoritative.
func parseVibeLine(line string) []Progress {
	var ev vibeStreamEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return nil
	}
	switch ev.kind() {
	case "message", "assistant", "assistant.message", "chat":
		if b := ev.body(); b != "" {
			return []Progress{{Kind: a2a.EventMessage, Message: &a2a.MessagePayload{
				Role: "assistant",
				Text: b,
			}}}
		}
	case "tool", "tool.call", "tool.use":
		return []Progress{{Kind: a2a.EventTool, Tool: &a2a.ToolPayload{
			Name: ev.Tool,
		}}}
	case "usage", "token_usage":
		return nil // usage fields unparsed until ISI-6204 pins the shape
	case "error":
		return []Progress{{Kind: a2a.EventMessage, Message: &a2a.MessagePayload{
			Role: "assistant",
			Text: "vibe error: " + ev.Error,
		}}}
	}
	return nil
}

// vibeSettleLine reports whether one stdout line is the stream's terminal
// marker: the CLI's signal that the session's work is done. Once seen (and
// the stream then stays quiet for the runner's settle window) the runner
// force-terminates the process and settles the task completed — the same
// contract as opencode's step_finish (a pinned CLI can leave non-daemon
// handles holding its event loop open after the work finished).
//
// NOTE(ISI-6203): recognized markers are the result/done family; ISI-6204
// pins the exact py-v0.3.0 terminal marker on a vanilla kind install.
func vibeSettleLine(line string) bool {
	var ev vibeStreamEvent
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		return false
	}
	switch ev.kind() {
	case "result", "done", "complete", "completed", "finish", "finished":
		return true
	}
	return false
}
