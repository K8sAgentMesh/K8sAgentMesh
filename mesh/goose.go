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

package mesh

import (
	"fmt"
	"path/filepath"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
)

// The goose flavor env contract, locked by the ISI-6128 Phase A spike
// (docs/isi-6128-goose-integration-spike.md @ 2b9a1ed): every name below was
// verified against the pinned v1.55.0 static binary, not assumed.
const (
	// gooseProviderEnv / gooseModelEnv select goose's provider lane and model
	// route. goose is multi-provider (openai, anthropic, ollama, …), so unlike
	// the single-wire runtimes the provider is part of the flavor env.
	gooseProviderEnv = "GOOSE_PROVIDER"
	gooseModelEnv    = "GOOSE_MODEL"
	// gooseModeEnv sets goose's approval semantics. The spike decision pins
	// auto — tools execute without interactive approval — because the sandbox
	// has no human to answer a permission prompt; the card therefore advertises
	// InteractivePrompt: false.
	gooseModeEnv = "GOOSE_MODE"
	// gooseModeHeadless is the locked headless value for gooseModeEnv.
	gooseModeHeadless = "auto"
	// gooseTelemetryOffEnv / gooseDisableKeyringEnv are the mandatory sandbox
	// knobs from the spike's env contract: no telemetry emission, and no
	// keyring access (headless sandboxes have no keyring; goose falls back to
	// its secrets.yaml file inside the isolated config dir).
	gooseTelemetryOffEnv   = "GOOSE_TELEMETRY_OFF"
	gooseDisableKeyringEnv = "GOOSE_DISABLE_KEYRING"
	// gooseOpenAIProvider is the provider lane a BYO OpenAI-compatible
	// ModelRoute rides: goose's openai provider consumes OPENAI_BASE_URL /
	// OPENAI_API_KEY (verified present in the v1.55.0 binary), which is the
	// same shape adapters #1/#2 stamp for their BYO routes.
	gooseOpenAIProvider = "openai"
	// gooseConfigDirEnv isolates goose's state directory. v1.55.0 has NO
	// GOOSE_CONFIG_DIR (verified against the binary: zero occurrences); goose
	// resolves its config dir through the XDG base-dir strategy, so
	// XDG_CONFIG_HOME is the evidence-backed isolation knob.
	gooseConfigDirEnv = "XDG_CONFIG_HOME"
	// gooseConfigDirName is the per-process XDG config root derived under the
	// process's launch dir; goose nests its own config under <root>/goose
	// (config.yaml, secrets.yaml, sessions.db).
	gooseConfigDirName = ".config"
)

// NewGooseAdapter builds adapter #4 (chapter §9.3 P4.2, ISI-6197 — the mesh
// half of ISI-6128 Phase B): Block's goose CLI behind the same 7-op
// RuntimeAdapter seam. Like adapters #1/#2/#3 it lands as a NEW ADAPTER ONLY —
// the gateway, router, lifecycle and supervisor keep speaking RuntimeAdapter
// and never see "goose". That zero-control-plane property is the standing
// acceptance test the opencode/codex seam established (P2) and the vibe
// adapter re-proved (P4.1); a fourth runtime exercising it with no
// control-plane diff is the P4.2 mesh-side proof.
//
// Per the locked Phase A spike decision, the standing process is `shim
// supervisor` with the goose flavor stamped into its env on the shared
// ChildProcessAdapter base; the supervisor lazily builds its goose engine on
// the first routed task, and the per-task surface is one-shot `goose run -i -
// --output-format stream-json --no-session --no-profile` (NDJSON with a
// terminal `complete` event). ACP (`goose acp` / `goose serve`) is proven but
// deliberately deferred — re-opened only if upstream deprecates headless run.
//
// host is the substrate the process runs on (LocalHost for the prototype, a
// kube sandbox-pod host later — whose image carries the pinned static musl
// goose binary; the CLI pin is re-verified on every bump because upstream is
// mid-migration, repo moved block/goose → aaif-goose/goose).
func NewGooseAdapter(host PlatformHost, spec RuntimeSpec, opts ...Option) (RuntimeAdapter, error) {
	if host == nil {
		return nil, fmt.Errorf("mesh: NewGooseAdapter requires a PlatformHost")
	}
	if err := spec.validate(); err != nil {
		return nil, fmt.Errorf("mesh: goose runtime spec: %w", err)
	}
	return newChildProcessAdapter(
		host,
		spec.Type,
		gooseCard(spec),
		gooseLaunch(spec),
		opts...,
	), nil
}

// gooseCard returns the agent-card builder for goose. It mirrors the shim
// engine's own card construction, sourcing the honest capability set, default
// model and credential shape from the injected runtime spec so the mesh never
// advertises what the process cannot serve. The shape is identical to adapters
// #1/#2/#3 by design — the card is a pure function of the runtime spec plus
// the envelope, and goose differs only in the values that spec supplies: the
// CLI pin (1.55.0), the multi-provider default model, the api-key credential
// shape, and a capability set that is exactly the spike-proven surface —
// streaming (stream-json), tool calls, BYO model endpoint, and NO interactive
// approval (GOOSE_MODE=auto is headless); the unproven extras (docker, github,
// package install) stay unadvertised gaps until the shim-side engine
// enumerates goose's tool-loop vocabulary.
func gooseCard(spec RuntimeSpec) func(Envelope) a2a.AgentCard {
	return func(env Envelope) a2a.AgentCard {
		model := spec.DefaultModel
		if env.Model != "" {
			model.ID = env.Model // context window stays the runtime's authority
		}
		skills := env.Skills
		if skills == nil {
			skills = []string{}
		}
		return a2a.AgentCard{
			SchemaVersion: spec.schemaVersion(),
			Agent: a2a.AgentIdentity{
				Name:    env.Identity.Name,
				Squad:   env.Identity.Squad,
				Project: env.Identity.Project,
			},
			Runtime: a2a.RuntimeInfo{
				Type:        spec.Type,
				CLIVersion:  spec.CLIVersion,
				ShimVersion: shimVersion,
			},
			Model:  model,
			Skills: skills,
			Auth: a2a.AuthInfo{
				Type: spec.CredentialType,
			},
			Capabilities: spec.Capabilities,
			Protocol:     spec.Protocol,
		}
	}
}

// gooseLaunch returns the LaunchSpec builder for goose. The standing process
// is `shim supervisor` with the flavor stamped into its env via the injected
// env contract; the supervisor lazily builds its goose engine on the first
// routed task. Beyond the shared shape, three goose-specific halves ride the
// standing-process env from the start:
//
//   - The locked headless contract: GOOSE_MODE=auto (no interactive
//     approval), GOOSE_TELEMETRY_OFF=1, GOOSE_DISABLE_KEYRING=1 — the sandbox
//     cannot answer a prompt, emit telemetry, or reach a keyring.
//   - The model pin: GOOSE_MODEL carries the envelope's override, else the
//     runtime default, so the process serves the role's model from the first
//     task; GOOSE_PROVIDER follows the lane — the BYO OpenAI-compatible
//     ModelRoute rides goose's openai lane (OPENAI_BASE_URL/OPENAI_API_KEY,
//     the placeholder key on the zero-credential Ollama-compatible lane),
//     unless the caller pinned its own provider (e.g. the native ollama lane
//     with OLLAMA_HOST via env). With neither, no provider is stamped: the
//     host's rendered config.yaml carries its default lane and the adapter
//     never guesses one.
//   - The isolated config dir: goose parks config.yaml, secrets.yaml and
//     sessions.db under its XDG config dir, so N standing processes on one
//     host must not share it (spike risk). v1.55.0 has no GOOSE_CONFIG_DIR;
//     the evidence-backed knob is XDG_CONFIG_HOME, pointed inside the
//     process's own launch dir (plain WorkDir, or the ADR-0024 §1 worktree
//     when the source layout is set). A caller-provided XDG_CONFIG_HOME wins,
//     and no dir is derived when none is known — a dedicated sandbox pod's
//     per-pod HOME is already isolated.
//
// Caller-supplied env (a resolved credential, operator OTLP, …) wins over the
// defaults, and the env is built deterministically (sorted) so a given
// envelope yields a byte-identical spec — the property Recreate relies on for
// its bare-exec rebuild. The per-process source layout overrides the plain
// WorkDir the same way it does for adapters #1/#2/#3.
func gooseLaunch(spec RuntimeSpec) func(Envelope) (LaunchSpec, error) {
	return func(env Envelope) (LaunchSpec, error) {
		procEnv := map[string]string{
			spec.RuntimeTypeEnv: spec.Type,
			// The locked headless contract (spike decision): the sandbox has no
			// human, no telemetry egress and no keyring.
			gooseModeEnv:           gooseModeHeadless,
			gooseTelemetryOffEnv:   "1",
			gooseDisableKeyringEnv: "1",
		}
		// Pin the model the headless process serves. The envelope override
		// wins; otherwise the runtime default the host advertised on the card.
		model := spec.DefaultModel.ID
		if env.Model != "" {
			model = env.Model
		}
		if model != "" {
			procEnv[gooseModelEnv] = model
		}
		// Provider lane: the caller's pin wins (checked before the defaults
		// loop below, which would otherwise overwrite it); otherwise a BYO
		// OpenAI-compatible route selects goose's openai lane. With neither,
		// nothing is stamped — the host's config.yaml default lane applies.
		_, callerPinnedProvider := env.Env[gooseProviderEnv]
		if !callerPinnedProvider && env.ModelRoute.Endpoint != "" {
			procEnv[gooseProviderEnv] = gooseOpenAIProvider
		}
		if env.ModelRoute.Endpoint != "" {
			procEnv["OPENAI_BASE_URL"] = env.ModelRoute.Endpoint
			key := env.ModelRoute.Token
			if key == "" {
				key = "ollama" // the zero-credential Ollama lane placeholder
			}
			procEnv["OPENAI_API_KEY"] = key
		}
		for k, v := range env.Env {
			procEnv[k] = v
		}
		launch := LaunchSpec{
			RuntimeType: spec.Type,
			Env:         nil, // rendered below, after the config-dir derivation
			WorkDir:     env.WorkDir,
			Identity:    env.Identity,
			Source:      env.Source,
		}
		// Per-process write discipline (ISI-6009, ADR-0024 §1): the source
		// layout wins over the plain WorkDir — the process runs in its own
		// worktree.
		if env.Source != nil {
			launch.WorkDir = env.Source.WorktreeDir
		}
		// Isolated config dir: derive XDG_CONFIG_HOME inside the resolved
		// launch dir unless the caller already set one (a kube host with
		// per-pod HOME may prefer the default).
		if _, callerSetConfigDir := env.Env[gooseConfigDirEnv]; !callerSetConfigDir && launch.WorkDir != "" {
			procEnv[gooseConfigDirEnv] = filepath.Join(launch.WorkDir, gooseConfigDirName)
		}
		launch.Env = sortedEnv(procEnv)
		return launch, nil
	}
}
