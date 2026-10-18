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

// Package taskrun is the shared per-task execution core of the vanilla
// runtime substrate (ISI-6217): it turns one normalized task into a
// registry-pinned ExecSpec run, tailing the CLI's stdout into the
// flavor-neutral progress Event wire and enforcing the settle contract
// (terminal marker seen + quiet window, then force-terminate — a pinned CLI
// can leave non-daemon handles holding its event loop open).
//
// Two front-ends ride this core, one wire each:
//
//	cmd/agentruntime  the stdio supervisor — one taskRequest JSON per stdin
//	                  line, one Event per stdout line (the one-shot surface
//	                  the live-validation lane drives).
//	cmd/mesh-supervisor the standing HTTP supervisor — one a2a.Task per
//	                  POST /task, Events translated onto the a2a.Event
//	                  NDJSON stream the mesh httpRouter tails (the ingest
//	                  surface the chart ships).
package taskrun

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
)

// Settle contract knobs, single-sourced from the former cmd/agentruntime
// implementation (ISI-4224 lineage).
const (
	// SettleWindow is how long the stream must stay quiet after the CLI's
	// terminal marker before the run is force-terminated.
	SettleWindow = 5 * time.Second
	// CommandTimeout bounds one task's agent loop wall clock.
	CommandTimeout = 30 * time.Minute
)

// Request is the normalized per-task input both supervisor front-ends
// resolve their wire into before execution.
type Request struct {
	// ID is the task identifier echoed on every Event (a2a_task_id on the
	// HTTP wire, id on the stdio wire).
	ID string
	// Envelope carries the system context + work instruction (spec §8.5).
	Envelope a2a.Envelope
	// Model is an optional model override (Agent.spec.model; empty uses the
	// runtime default).
	Model string
	// WorkDir is the directory the CLI runs in; the caller owns allocation
	// (a per-task temp dir on the standing path, the routed workDir on the
	// stdio path).
	WorkDir string
	// ModelRoute is the resolved BYO OpenAI-compatible route (spec §11).
	ModelRoute a2a.ModelRoute
}

// Event is the flavor-neutral progress wire: exactly the JSON shape
// cmd/agentruntime emits per stdout line. kind is "message", "tool",
// "completed" or "failed"; a terminal Event ends the task (the caller closes
// its stream right after).
type Event struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
	Exit    int    `json:"exit,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Terminal reports whether e settles the task.
func (e Event) Terminal() bool {
	return e.Kind == "completed" || e.Kind == "failed"
}

// CredentialEnv names the process env var the runtime reads its credential
// from (credentials NEVER ride the task channel — the standing-process
// contract).
func CredentialEnv(rt agentruntime.Runtime) string {
	switch rt.Type() {
	case agentruntime.RuntimeTypeVibe:
		return "MISTRAL_API_KEY"
	case agentruntime.RuntimeTypeCodex:
		return "OPENAI_API_KEY"
	case agentruntime.RuntimeTypeGoose:
		// The openai BYO lane's key (ISI-6128 spike contract; goose's native
		// lanes read their own env from the host config).
		return "OPENAI_API_KEY"
	}
	return ""
}

// Run executes one task through the registry-pinned runtime, feeding every
// progress Event to emit (never nil; exactly one terminal Event is emitted
// before Run returns). ctx governs the run; the caller's cancellation
// propagates to the CLI via exec.CommandContext.
func Run(ctx context.Context, rt agentruntime.Runtime, task Request, emit func(Event)) {
	spec, err := rt.Command(agentruntime.LaunchContext{
		Envelope:   task.Envelope,
		ModelRoute: task.ModelRoute,
		Model:      task.Model,
		Credential: os.Getenv(CredentialEnv(rt)),
		WorkDir:    task.WorkDir,
	})
	if err != nil {
		emit(Event{Kind: "failed", ID: task.ID, Error: err.Error()})
		return
	}

	tctx, cancel := context.WithTimeout(ctx, CommandTimeout)
	defer cancel()
	// The path comes from the pinned agentruntime engine registry
	// (never the task channel).
	cmd := exec.CommandContext(tctx, spec.Path, spec.Args...) // #nosec G204 -- registry-pinned engine path
	cmd.Dir = spec.WorkDir
	cmd.Env = append(os.Environ(), spec.Env...)
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	for _, f := range spec.WorkDirFiles {
		// Rendered MCP config: credentials stay in process env.
		// #nosec G306 -- config is secret-free by construction (ADR-045 D5)
		if err := os.WriteFile(joinPath(spec.WorkDir, f.Name), f.Content, 0o600); err != nil {
			emit(Event{Kind: "failed", ID: task.ID, Error: "workdir file: " + err.Error()})
			return
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		emit(Event{Kind: "failed", ID: task.ID, Error: err.Error()})
		return
	}
	if err := cmd.Start(); err != nil {
		emit(Event{Kind: "failed", ID: task.ID, Error: err.Error()})
		return
	}

	// Settle contract: once the CLI's terminal marker was seen and the
	// stream then stays quiet for the settle window, force-terminate — a
	// pinned CLI can leave non-daemon handles holding its event loop open.
	var settledAt time.Time
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if spec.Parse != nil {
			for _, p := range spec.Parse(line) {
				switch {
				case p.Message != nil:
					emit(Event{Kind: "message", ID: task.ID, Role: p.Message.Role, Content: p.Message.Text})
				case p.Tool != nil:
					emit(Event{Kind: "tool", ID: task.ID, Name: p.Tool.Name})
				}
			}
		} else if line != "" {
			emit(Event{Kind: "message", ID: task.ID, Role: "assistant", Content: line})
		}
		if spec.SettleLine != nil && spec.SettleLine(line) {
			settledAt = time.Now()
		}
		if !settledAt.IsZero() && time.Since(settledAt) > SettleWindow {
			_ = cmd.Process.Kill()
			break
		}
	}
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()

	exit := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			emit(Event{Kind: "failed", ID: task.ID, Error: waitErr.Error()})
			return
		}
	}
	if !settledAt.IsZero() {
		exit = 0 // the terminal marker settles the run regardless of lingering handles
	}
	emit(Event{Kind: "completed", ID: task.ID, Exit: exit})
}

func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}
