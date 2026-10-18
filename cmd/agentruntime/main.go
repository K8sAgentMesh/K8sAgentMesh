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

// Command agentruntime is the per-task STDIO supervisor of the standalone
// mesh deploy (ISI-6158 C3): it serves the /task NDJSON contract — one JSON
// task per stdin line, one JSON progress event per stdout line — on top of
// the agentruntime registry. The wire shape is the one the k8squad shim
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
//
// The execution core (registry-pinned ExecSpec run, stream parse, settle
// window) lives in internal/taskrun and is shared with cmd/mesh-supervisor,
// the HTTP front-end the chart ships as the deployed ingest surface
// (ISI-6217 gap 2): this binary stays the one-shot stdio surface a
// Job-per-task lane (or a piped container) drives directly.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
	"github.com/K8sAgentMesh/K8sAgentMesh/internal/taskrun"
)

// runtimeTypeEnv is the standing-process flavor contract (wire-compatible
// with the k8squad supervisor; see agentruntime doc).
const runtimeTypeEnv = "KSQUAD_RUNTIME_TYPE"

// taskRequest is the stdio task wire (one JSON object per stdin line).
type taskRequest struct {
	ID            string         `json:"id"`
	Input         string         `json:"input"`
	SystemContext string         `json:"systemContext"`
	Model         string         `json:"model"`
	WorkDir       string         `json:"workDir"`
	ModelRoute    a2a.ModelRoute `json:"modelRoute"`
}

// emit writes one progress Event as a stdout NDJSON line.
func emit(ev taskrun.Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	fmt.Println(string(b))
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
			emit(taskrun.Event{Kind: "failed", Error: "bad task line: " + err.Error()})
			continue
		}
		taskrun.Run(ctx, rt, taskrun.Request{
			ID:         task.ID,
			Envelope:   a2a.Envelope{SystemContext: task.SystemContext, Input: task.Input},
			Model:      task.Model,
			WorkDir:    task.WorkDir,
			ModelRoute: task.ModelRoute,
		}, emit)
	}
}
