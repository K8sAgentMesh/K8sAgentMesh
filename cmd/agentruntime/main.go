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

// Command agentruntime is the per-task supervisor of the standalone mesh
// deploy (ISI-6158 C3): it serves the /task NDJSON contract — one JSON task
// per stdin line, one JSON progress event per stdout line — on top of the
// agentruntime registry. The wire shape is the one the k8squad shim
// supervisor already speaks; a vendored host swap changes the engine
// source, not the contract.
//
// The runtime flavor is stamped into the pod env (KSQUAD_RUNTIME_TYPE, the
// standing-process contract); credentials NEVER ride the task JSON — the
// supervisor reads them from the process env (MISTRAL_API_KEY /
// OPENAI_API_KEY), keeping secret material off the task channel.
//
//	Task line:  {"id":"...","input":"...","systemContext":"...","model":"...",
//	             "workDir":"/wt/42","modelRoute":{"endpoint":"...","model":"..."}}
//	Progress:   {"kind":"message","role":"assistant","content":"..."}
//	            {"kind":"tool","name":"shell"}
//	            {"kind":"completed","exit":0,"id":"..."}
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
)

const (
	runtimeTypeEnv = "KSQUAD_RUNTIME_TYPE"
	settleWindow   = 5 * time.Second
	commandTimeout = 30 * time.Minute
)

type taskRequest struct {
	ID            string         `json:"id"`
	Input         string         `json:"input"`
	SystemContext string         `json:"systemContext"`
	Model         string         `json:"model"`
	WorkDir       string         `json:"workDir"`
	ModelRoute    a2a.ModelRoute `json:"modelRoute"`
}

type progressEvent struct {
	Kind    string `json:"kind"`
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
	Exit    int    `json:"exit,omitempty"`
	Error   string `json:"error,omitempty"`
}

func emit(ev progressEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Println(string(b))
}

func credentialEnv(rt agentruntime.Runtime) string {
	switch rt.Type() {
	case agentruntime.RuntimeTypeVibe:
		return "MISTRAL_API_KEY"
	case agentruntime.RuntimeTypeCodex:
		return "OPENAI_API_KEY"
	}
	return ""
}

func runTask(ctx context.Context, rt agentruntime.Runtime, task taskRequest) {
	spec, err := rt.Command(agentruntime.LaunchContext{
		Envelope: a2a.Envelope{
			SystemContext: task.SystemContext,
			Input:         task.Input,
		},
		ModelRoute: task.ModelRoute,
		Model:      task.Model,
		Credential: os.Getenv(credentialEnv(rt)),
		WorkDir:    task.WorkDir,
	})
	if err != nil {
		emit(progressEvent{Kind: "failed", ID: task.ID, Error: err.Error()})
		return
	}

	tctx, cancel := context.WithTimeout(ctx, commandTimeout)
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
			emit(progressEvent{Kind: "failed", ID: task.ID, Error: "workdir file: " + err.Error()})
			return
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		emit(progressEvent{Kind: "failed", ID: task.ID, Error: err.Error()})
		return
	}
	if err := cmd.Start(); err != nil {
		emit(progressEvent{Kind: "failed", ID: task.ID, Error: err.Error()})
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
					emit(progressEvent{Kind: "message", ID: task.ID, Role: p.Message.Role, Content: p.Message.Text})
				case p.Tool != nil:
					emit(progressEvent{Kind: "tool", ID: task.ID, Name: p.Tool.Name})
				}
			}
		} else if line != "" {
			emit(progressEvent{Kind: "message", ID: task.ID, Role: "assistant", Content: line})
		}
		if spec.SettleLine != nil && spec.SettleLine(line) {
			settledAt = time.Now()
		}
		if !settledAt.IsZero() && time.Since(settledAt) > settleWindow {
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
			emit(progressEvent{Kind: "failed", ID: task.ID, Error: waitErr.Error()})
			return
		}
	}
	if !settledAt.IsZero() {
		exit = 0 // the terminal marker settles the run regardless of lingering handles
	}
	emit(progressEvent{Kind: "completed", ID: task.ID, Exit: exit})
}

func joinPath(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + string(os.PathSeparator) + name
}

func main() {
	runtimeType := os.Getenv(runtimeTypeEnv)
	if runtimeType == "" {
		fmt.Fprintln(os.Stderr, "agentruntime: KSQUAD_RUNTIME_TYPE is required (the standing-process flavor contract)")
		os.Exit(1)
	}
	rt, err := agentruntime.Get(runtimeType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentruntime:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var task taskRequest
		if err := json.Unmarshal([]byte(line), &task); err != nil {
			emit(progressEvent{Kind: "failed", Error: "bad task line: " + err.Error()})
			continue
		}
		runTask(ctx, rt, task)
	}
}
