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

// Command mesh-gateway runs the K8sAgentMesh standing-process gateway as a
// standalone pod binary (ISI-6158 C3): it builds one runtime adapter from
// the agentruntime registry, wires it into mesh.NewGateway and serves the
// standing band until the pod terminates.
//
// Configuration is env-driven (the helm chart supplies it):
//
//	MESH_RUNTIME_TYPE  runtime flavor (vibe|codex|opencode|goose; default vibe)
//	MESH_MODEL         envelope model override (optional)
//	MESH_AGENT_NAME    card identity name (default mesh-gateway)
//	MESH_POOL_MIN / MESH_POOL_MAX  standing band shape (default 1/1)
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/K8sAgentMesh/K8sAgentMesh/a2a"
	"github.com/K8sAgentMesh/K8sAgentMesh/agentruntime"
	"github.com/K8sAgentMesh/K8sAgentMesh/mesh"
)

// runtimeTypeEnv is the standing-process flavor contract (wire-compatible
// with the k8squad supervisor; see agentruntime doc).
const runtimeTypeEnv = "KSQUAD_RUNTIME_TYPE"

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getenvInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func credentialType(rt agentruntime.Runtime) string {
	switch rt.Type() {
	case agentruntime.RuntimeTypeVibe:
		return agentruntime.CredentialTypeMistral
	case agentruntime.RuntimeTypeCodex:
		return agentruntime.CredentialTypeOpenAI
	case agentruntime.RuntimeTypeGoose:
		// api-key shape on the OpenAI-compatible BYO lane (ISI-6128 spike).
		return "api_key"
	}
	return "api_key"
}

func adapter(host mesh.PlatformHost, spec mesh.RuntimeSpec) (mesh.RuntimeAdapter, error) {
	switch spec.Type {
	case agentruntime.RuntimeTypeVibe:
		return mesh.NewVibeAdapter(host, spec)
	case agentruntime.RuntimeTypeCodex:
		return mesh.NewCodexAdapter(host, spec)
	case "opencode":
		return mesh.NewOpenCodeAdapter(host, spec)
	case "goose":
		return mesh.NewGooseAdapter(host, spec)
	}
	return nil, fmt.Errorf("mesh-gateway: no adapter constructor for runtime type %q", spec.Type)
}

func main() {
	runtimeType := getenv("MESH_RUNTIME_TYPE", agentruntime.RuntimeTypeVibe)
	rt, err := agentruntime.Get(runtimeType)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mesh-gateway:", err)
		os.Exit(1)
	}
	spec := mesh.RuntimeSpec{
		Type:           rt.Type(),
		CLIVersion:     rt.CLIVersion(),
		Capabilities:   rt.Capabilities(),
		DefaultModel:   rt.DefaultModel(),
		CredentialType: credentialType(rt),
		SchemaVersion:  a2a.DefaultSchemaVersion,
		Protocol:       a2a.ProtocolVersions{A2A: "0.2.0", MCP: "2025-06-18"},
		RuntimeTypeEnv: runtimeTypeEnv,
	}

	ad, err := adapter(&mesh.LocalHost{}, spec)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mesh-gateway:", err)
		os.Exit(1)
	}

	envelope := mesh.Envelope{
		Identity: a2a.AgentIdentity{Name: getenv("MESH_AGENT_NAME", "mesh-gateway")},
	}
	if m := os.Getenv("MESH_MODEL"); m != "" {
		envelope.Model = m
	}

	gw, err := mesh.NewGateway(mesh.GatewayConfig{
		Adapter:  ad,
		Envelope: envelope,
		Min:      getenvInt("MESH_POOL_MIN", 1),
		Max:      getenvInt("MESH_POOL_MAX", 1),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mesh-gateway:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := gw.Start(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "mesh-gateway:", err)
		os.Exit(1)
	}
}
