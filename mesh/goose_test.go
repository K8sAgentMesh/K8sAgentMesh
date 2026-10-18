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
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
)

// testGooseType is the runtime flavor name the module tests inject for goose
// (a host injects its own constant; k8squad host: api/v1alpha1
// RuntimeTypeGoose).
const testGooseType = "goose"

// testGooseSpec is the injected runtime profile standing in for a host's
// declarative goose runtime — plain values, exactly what a host plugin
// supplies at wiring time (k8squad host: pkg/shim/runtimes +
// internal/protocol + its SchemaVersion stamp). The pinned CLIVersion mirrors
// the spike's binary pin, the credential shape is the api-key form the
// OpenAI-compatible BYO lanes consume, and the capability set is exactly the
// spike-proven surface — streaming, tool calls, BYO endpoint, NO interactive
// approval — with the unproven extras left as advertised gaps.
func testGooseSpec() RuntimeSpec {
	return RuntimeSpec{
		Type:           testGooseType,
		CLIVersion:     "1.55.0",
		Capabilities:   a2a.Capabilities{Streaming: true, ToolCalls: true, InteractivePrompt: false, BYOModelEndpoint: true},
		DefaultModel:   a2a.ModelInfo{ID: "qwen3.5:35b", ContextWindow: 131072},
		CredentialType: "apikey",
		SchemaVersion:  a2a.DefaultSchemaVersion,
		Protocol:       a2a.ProtocolVersions{A2A: "0.2.0", MCP: "2025-06-18"},
		RuntimeTypeEnv: testRuntimeTypeEnv,
	}
}

func TestNewGooseAdapterRequiresHost(t *testing.T) {
	if _, err := NewGooseAdapter(nil, testGooseSpec()); err == nil {
		t.Fatal("NewGooseAdapter must reject a nil host")
	}
}

// The injected-spec seam fails loudly at wiring time (the injected analogue of
// the host registry's "runtime not registered"), never mid-Spawn.
func TestNewGooseAdapterRequiresValidSpec(t *testing.T) {
	spec := testGooseSpec()
	spec.Type = ""
	if _, err := NewGooseAdapter(&fakeHost{}, spec); err == nil {
		t.Fatal("NewGooseAdapter must reject an empty runtime type")
	}
	spec = testGooseSpec()
	spec.RuntimeTypeEnv = ""
	if _, err := NewGooseAdapter(&fakeHost{}, spec); err == nil {
		t.Fatal("NewGooseAdapter must reject an empty launch env contract")
	}
}

func TestGooseSpawnStampsRuntimeAndCard(t *testing.T) {
	host := &fakeHost{}
	spec := testGooseSpec()
	a, err := NewGooseAdapter(host, spec)
	if err != nil {
		t.Fatalf("NewGooseAdapter: %v", err)
	}

	_, card, err := a.Spawn(context.Background(), Envelope{
		Skills:   []string{"code-review"},
		Identity: a2a.AgentIdentity{Name: "coder", Squad: "eng", Project: "k8squad"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if len(host.specs) != 1 {
		t.Fatalf("expected one launch, got %d", len(host.specs))
	}
	if host.specs[0].RuntimeType != spec.Type {
		t.Fatalf("launch runtime type = %q, want %q", host.specs[0].RuntimeType, spec.Type)
	}
	if !hasEnv(host.specs[0].Env, spec.RuntimeTypeEnv+"="+spec.Type) {
		t.Fatalf("launch env %v missing %s=%s", host.specs[0].Env, spec.RuntimeTypeEnv, spec.Type)
	}
	// The locked headless contract from the spike decision: no interactive
	// approval, no telemetry, no keyring — stamped on every goose launch.
	if !hasEnv(host.specs[0].Env, gooseModeEnv+"="+gooseModeHeadless) {
		t.Fatalf("launch env %v missing %s=%s (headless lock)", host.specs[0].Env, gooseModeEnv, gooseModeHeadless)
	}
	if !hasEnv(host.specs[0].Env, gooseTelemetryOffEnv+"=1") {
		t.Fatalf("launch env %v missing %s=1", host.specs[0].Env, gooseTelemetryOffEnv)
	}
	if !hasEnv(host.specs[0].Env, gooseDisableKeyringEnv+"=1") {
		t.Fatalf("launch env %v missing %s=1", host.specs[0].Env, gooseDisableKeyringEnv)
	}
	// The model pin: the standing process serves the runtime default model
	// from the first task when the envelope carries no override.
	if !hasEnv(host.specs[0].Env, gooseModelEnv+"="+spec.DefaultModel.ID) {
		t.Fatalf("launch env %v missing %s=%s (default-model pin)", host.specs[0].Env, gooseModelEnv, spec.DefaultModel.ID)
	}

	// The card must mirror the injected runtime spec's honest contract — the
	// spike-proven capabilities only, interactive approval advertised as a
	// gap (GOOSE_MODE=auto is headless).
	if card.Runtime.Type != spec.Type {
		t.Fatalf("card runtime type = %q, want %q", card.Runtime.Type, spec.Type)
	}
	if card.Runtime.CLIVersion != spec.CLIVersion {
		t.Fatalf("card CLI version = %q, want %q", card.Runtime.CLIVersion, spec.CLIVersion)
	}
	if card.Model.ID != spec.DefaultModel.ID {
		t.Fatalf("card model = %q, want default %q", card.Model.ID, spec.DefaultModel.ID)
	}
	if !reflect.DeepEqual(card.Capabilities, spec.Capabilities) {
		t.Fatalf("card capabilities = %+v, want %+v", card.Capabilities, spec.Capabilities)
	}
	if card.Capabilities.InteractivePrompt {
		t.Fatal("goose card must not advertise interactive approval — GOOSE_MODE=auto is headless")
	}
	if card.Auth.Type != spec.CredentialType {
		t.Fatalf("card auth shape = %q, want %q", card.Auth.Type, spec.CredentialType)
	}
	if card.Agent.Name != "coder" || card.Agent.Project != "k8squad" {
		t.Fatalf("card identity = %+v, want coder/k8squad", card.Agent)
	}
	if card.SchemaVersion != spec.SchemaVersion {
		t.Fatalf("card schema = %q, want %q", card.SchemaVersion, spec.SchemaVersion)
	}
	if card.Protocol != spec.Protocol {
		t.Fatalf("card protocol = %+v, want %+v", card.Protocol, spec.Protocol)
	}
}

func TestGooseModelOverridePinsModel(t *testing.T) {
	host := &fakeHost{}
	spec := testGooseSpec()
	a, _ := NewGooseAdapter(host, spec)
	_, card, err := a.Spawn(context.Background(), Envelope{Model: "qwen3.5:122b"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if card.Model.ID != "qwen3.5:122b" {
		t.Fatalf("card model = %q, want the override qwen3.5:122b", card.Model.ID)
	}
	// The context window stays the runtime's authority, never the override's.
	if card.Model.ContextWindow != spec.DefaultModel.ContextWindow {
		t.Fatalf("context window = %d, want runtime default %d", card.Model.ContextWindow, spec.DefaultModel.ContextWindow)
	}
	// The override must also pin the standing process's GOOSE_MODEL so the
	// first task serves the role's model, not goose's own default.
	if !hasEnv(host.specs[0].Env, gooseModelEnv+"=qwen3.5:122b") {
		t.Fatalf("launch env %v missing %s override pin", host.specs[0].Env, gooseModelEnv)
	}
}

// A BYO OpenAI-compatible route rides goose's openai lane — the same
// OPENAI_BASE_URL/OPENAI_API_KEY shape adapters #1/#2 stamp, with the
// placeholder key on the zero-credential lane.
func TestGooseBYOModelRouteEnv(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	_, _, err := a.Spawn(context.Background(), Envelope{
		ModelRoute: a2a.ModelRoute{Endpoint: "http://ollama:11434/v1", Model: "qwen3"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	env := host.specs[0].Env
	if !hasEnv(env, gooseProviderEnv+"="+gooseOpenAIProvider) {
		t.Fatalf("env %v missing %s=%s (BYO route lane)", env, gooseProviderEnv, gooseOpenAIProvider)
	}
	if !hasEnv(env, "OPENAI_BASE_URL=http://ollama:11434/v1") {
		t.Fatalf("env %v missing OPENAI_BASE_URL", env)
	}
	if !hasEnv(env, "OPENAI_API_KEY=ollama") {
		t.Fatalf("env %v missing the ollama placeholder key", env)
	}
}

func TestGooseEnvOverrideWins(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	_, _, err := a.Spawn(context.Background(), Envelope{
		ModelRoute: a2a.ModelRoute{Endpoint: "http://e/v1"},
		Env:        map[string]string{"OPENAI_API_KEY": "sk-real"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	env := host.specs[0].Env
	if !hasEnv(env, "OPENAI_API_KEY=sk-real") {
		t.Fatalf("caller env must override the placeholder; got %v", env)
	}
	if hasEnv(env, "OPENAI_API_KEY=ollama") {
		t.Fatalf("placeholder key must not survive a caller override; got %v", env)
	}
}

// A caller-pinned provider lane (here: goose's native ollama lane with
// OLLAMA_HOST) wins over the BYO route's default openai lane — the host, not
// the adapter, owns the provider choice when it says so explicitly.
func TestGooseProviderCallerPinWins(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	_, _, err := a.Spawn(context.Background(), Envelope{
		ModelRoute: a2a.ModelRoute{Endpoint: "http://10.0.0.185:11434/v1"},
		Env:        map[string]string{gooseProviderEnv: "ollama", "OLLAMA_HOST": "http://10.0.0.185:11434"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	env := host.specs[0].Env
	if !hasEnv(env, gooseProviderEnv+"=ollama") {
		t.Fatalf("caller provider pin must survive; got %v", env)
	}
	if hasEnv(env, gooseProviderEnv+"="+gooseOpenAIProvider) {
		t.Fatalf("route default provider must not survive a caller pin; got %v", env)
	}
	if !hasEnv(env, "OLLAMA_HOST=http://10.0.0.185:11434") {
		t.Fatalf("caller OLLAMA_HOST must ride the launch env; got %v", env)
	}
}

// With no BYO route and no caller pin, no provider is stamped — the host's
// rendered config.yaml carries its default lane and the adapter never guesses
// one (the honest-gap rule: a wrong default would surface as a mid-Run
// failure, exactly what the spec exists to prevent).
func TestGooseNoProviderWithoutLane(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	if _, _, err := a.Spawn(context.Background(), Envelope{}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	for _, e := range host.specs[0].Env {
		if strings.HasPrefix(e, gooseProviderEnv+"=") {
			t.Fatalf("no provider lane may be stamped without a route or pin; got %v", host.specs[0].Env)
		}
	}
}

// The per-process config-dir isolation: goose parks config.yaml, secrets.yaml
// and sessions.db under its XDG config dir, so the adapter points
// XDG_CONFIG_HOME inside the process's own launch dir — the plain WorkDir, or
// the ADR-0024 §1 worktree when the source layout is set.
func TestGooseIsolatedConfigDir(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	if _, _, err := a.Spawn(context.Background(), Envelope{WorkDir: "/sandbox/proc-1"}); err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !hasEnv(host.specs[0].Env, gooseConfigDirEnv+"="+filepath.Join("/sandbox/proc-1", gooseConfigDirName)) {
		t.Fatalf("launch env %v missing isolated %s", host.specs[0].Env, gooseConfigDirEnv)
	}
}

func TestGooseIsolatedConfigDirFollowsSourceWorktree(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	_, _, err := a.Spawn(context.Background(), Envelope{
		WorkDir: "/sandbox/shared",
		Source:  &SourceSpec{Branch: "task-1", TeamMainDir: "/sandbox/main", WorktreeDir: "/sandbox/wt/task-1"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if !hasEnv(host.specs[0].Env, gooseConfigDirEnv+"="+filepath.Join("/sandbox/wt/task-1", gooseConfigDirName)) {
		t.Fatalf("launch env %v must isolate under the per-process worktree, not the shared WorkDir", host.specs[0].Env)
	}
}

// A caller-provided XDG_CONFIG_HOME wins over the derivation — a kube host
// with per-pod HOME may prefer the platform default.
func TestGooseConfigDirCallerOverrideWins(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	_, _, err := a.Spawn(context.Background(), Envelope{
		WorkDir: "/sandbox/proc-1",
		Env:     map[string]string{gooseConfigDirEnv: "/var/lib/goose"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	env := host.specs[0].Env
	if !hasEnv(env, gooseConfigDirEnv+"=/var/lib/goose") {
		t.Fatalf("caller config dir must survive; got %v", env)
	}
	if hasEnv(env, gooseConfigDirEnv+"="+filepath.Join("/sandbox/proc-1", gooseConfigDirName)) {
		t.Fatalf("derived config dir must not survive a caller override; got %v", env)
	}
}

func TestGooseLaunchEnvDeterministic(t *testing.T) {
	host := &fakeHost{}
	a, _ := NewGooseAdapter(host, testGooseSpec())
	env := Envelope{Env: map[string]string{"B": "2", "A": "1"}}
	if _, _, err := a.Spawn(context.Background(), env); err != nil {
		t.Fatalf("Spawn 1: %v", err)
	}
	if _, _, err := a.Spawn(context.Background(), env); err != nil {
		t.Fatalf("Spawn 2: %v", err)
	}
	if strings.Join(host.specs[0].Env, ",") != strings.Join(host.specs[1].Env, ",") {
		t.Fatalf("the same envelope must yield a byte-identical spec: %v vs %v", host.specs[0].Env, host.specs[1].Env)
	}
}

// The goose adapter must be usable purely through the RuntimeAdapter seam —
// the gateway never sees a concrete type (the "zero control-plane change"
// test a fourth runtime exercises at P4.2, per the locked spike decision).
func TestGooseIsRuntimeAdapter(t *testing.T) {
	if _, err := NewGooseAdapter(&fakeHost{}, testGooseSpec()); err != nil {
		t.Fatalf("NewGooseAdapter: %v", err)
	}
}
