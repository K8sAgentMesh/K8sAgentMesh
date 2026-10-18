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
	"strconv"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
)

// Single-sourced vibe constants (ISI-6158: the host registry entry values,
// now mesh-side per D1). The pinned CLI revision, default model and its
// context-window budget authority live here once and are asserted by unit
// tests; images/Dockerfile.toolchain-vibe installs exactly
// mistral-vibe==2.9.6 (ISI-6217: the former vibe==0.3.0 pin resolved against
// no real PyPI package — the CLI ships as mistral-vibe) and the two pins
// move together or not at all (ADR-017).
const (
	// vibeCLIVersion is the pinned headless `vibe` CLI revision the engine
	// targets (ADR-017 reproducibility): the mistral-vibe==2.9.6 PyPI pin.
	vibeCLIVersion = "py-v2.9.6"
	// vibeDefaultModel is Mistral's flagship model the vibe CLI serves.
	vibeDefaultModel = "mistral-large-latest"
	// vibeContextWindow is vibeDefaultModel's context-window budget
	// authority for the context Assembler (spec §6.2).
	vibeContextWindow = 131072
	// vibeMaxTurns bounds one task's agent loop (the CLI's --max-turns).
	vibeMaxTurns = 64
	// vibeActiveModelEnv pins the model the headless runtime serves: in
	// non-interactive (-p) mode the upstream CLI resolves its active model
	// through a GrowthBook feature flag that misfires without a TTY
	// (mistralai/mistral-vibe#1140). The mesh adapter pins it on the
	// standing process from start (mesh/vibe.go); this engine re-pins it
	// per task from the ROUTED model so an override holds.
	vibeActiveModelEnv = "VIBE_ACTIVE_MODEL"
)

// vibe is the Mistral headless coding-agent engine (ISI-6158): the per-task
// one-shot `vibe -p --output streaming` invocation in the codex-exec shape.
// The prompt rides stdin (never argv — NFR-SEC1, the same invariant as the
// opencode run seam); credentials map onto MISTRAL_API_KEY; a BYO
// OpenAI-compatible route rides MISTRAL_BASE_URL with the zero-credential
// "ollama" placeholder lane. The standing-process half of the runtime (the
// mesh ChildProcessAdapter the gateway manages) is mesh.NewVibeAdapter —
// this engine is the per-task half behind the supervisor.
type vibe struct{}

func init() { Register(vibe{}) }

func (vibe) Type() string                     { return RuntimeTypeVibe }
func (vibe) CLIVersion() string               { return vibeCLIVersion }
func (vibe) CredentialShape() CredentialShape { return ShapeAPIKey }

func (vibe) Capabilities() a2a.Capabilities {
	return a2a.Capabilities{
		Streaming:         true,
		ToolCalls:         true,
		InteractivePrompt: false, // headless -p only, by design
		BYOModelEndpoint:  true,  // MISTRAL_BASE_URL (OpenAI-compatible)
		ArtifactKinds:     []string{"file", "patch"},
		Docker:            true,
		GitHub:            true,
		PackageInstall:    true, // uv-managed toolchain
	}
}

func (vibe) DefaultModel() a2a.ModelInfo {
	return a2a.ModelInfo{ID: vibeDefaultModel, ContextWindow: vibeContextWindow}
}

func (r vibe) Command(lc LaunchContext) (ExecSpec, error) {
	// The model the task drives, preferring a BYO endpoint's model over the
	// Agent's fixed model, over the runtime default. Re-pinned per task so
	// the headless GrowthBook bug cannot silently drop the role off its
	// model (mistralai/mistral-vibe#1140) — the counterpart of the mesh
	// adapter's standing-process pin.
	model := resolveModel(r, lc)
	env := []string{}
	if model != "" {
		env = append(env, vibeActiveModelEnv+"="+model)
	}
	// vibe speaks the Mistral wire natively, so the per-user credential AND
	// a BYO model route both map onto MISTRAL_API_KEY. Emit exactly one: a
	// BYO endpoint carries its own token+base URL; without a route token
	// the zero-credential Ollama lane uses the conventional placeholder so
	// the client library still authenticates (spec §11/C9). Otherwise the
	// per-user credential is the Mistral key.
	key := lc.Credential
	if lc.ModelRoute.Endpoint != "" {
		env = append(env, "MISTRAL_BASE_URL="+lc.ModelRoute.Endpoint)
		switch {
		case lc.ModelRoute.Token != "":
			key = lc.ModelRoute.Token
		case key == "":
			key = "ollama"
		}
	}
	if key != "" {
		env = append(env, "MISTRAL_API_KEY="+key)
	}
	return ExecSpec{
		Path: "vibe",
		Args: []string{
			"-p",
			"--output", "streaming",
			"--auto-approve",
			"--workdir", lc.WorkDir,
			"--max-turns", strconv.Itoa(vibeMaxTurns),
			"--trust",
		},
		Env:     env,
		WorkDir: lc.WorkDir,
		// The whole envelope rides stdin (system context + instruction,
		// NFR-SEC1): vibe -p reads its prompt from stdin, keeping it out of
		// argv and the process table — the same seam as the opencode run.
		Stdin:      EnvelopePrompt(lc),
		Parse:      parseVibeLine,
		SettleLine: vibeSettleLine,
	}, nil
}
