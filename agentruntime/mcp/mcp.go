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

// Package mcp is the MCP intermediate-representation seam of the
// agentruntime engine layer: the projected Endpoint IR (secret-free by
// construction, ADR-045 D5) and the codex native-config renderer.
package mcp

import "fmt"

// SecretRef names a Secret + key without carrying material.
type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

// ObjectRef names an arbitrary API object (provenance only).
type ObjectRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// Endpoint is one projected MCP server (stdio or streamable-http).
type Endpoint struct {
	// Name is the MCPServer object name (also the server key in every
	// runtime-native config).
	Name string `json:"name"`
	// Transport is "stdio" or "streamable-http".
	Transport string `json:"transport"`
	// URL is the streamable-http endpoint (empty for stdio).
	URL string `json:"url,omitempty"`
	// Command is the stdio executable (empty for streamable-http).
	Command string `json:"command,omitempty"`
	// Args are the stdio command arguments.
	Args []string `json:"args,omitempty"`
	// Image is the stdio server's packaged image; non-empty means Run
	// assembly stages a native sidecar container running it.
	Image string `json:"image,omitempty"`
	// EnvNames are the environment variable NAMES the credential ref maps
	// to — never values. Rendered runtime configs reference these names so
	// secret material only ever lives in process env (ADR-045 D5).
	EnvNames []string `json:"envNames,omitempty"`
	// Headers are the static non-secret HTTP headers (streamable-http).
	Headers map[string]string `json:"headers,omitempty"`
	// AllowTools is the effective allow set.
	AllowTools []string `json:"allowTools,omitempty"`
	// DenyTools is the server envelope's deny globs, recorded verbatim.
	DenyTools []string `json:"denyTools,omitempty"`
	// CredentialSecretRef names the BYO Secret backing this server's
	// credential — a NAME, never material (ADR-045 D5).
	CredentialSecretRef *SecretRef `json:"credentialSecretRef,omitempty"`
	// EgressPolicyRef names the EgressPolicy covering a streamable-http
	// endpoint. Provenance only.
	EgressPolicyRef *ObjectRef `json:"egressPolicyRef,omitempty"`
}

// CredentialEnvName derives the deterministic env var name a server's
// credential is mounted as: KSQUAD_MCP_<UPPER(NAME)>_TOKEN. The env VALUE is
// a SecretKeyRef projection — the control plane never reads the Secret.
func CredentialEnvName(name string) string {
	out := make([]byte, 0, len(name)+len("KSQUAD_MCP__TOKEN"))
	out = append(out, "KSQUAD_MCP_"...)
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(append(out, "_TOKEN"...))
}

// RenderCodexConfig renders codex's config.toml from the projected MCP IR:
// the [mcp_servers.*] section plus, when a BYO endpoint is set, the
// [model_providers.ksquad-byo] block pointing the CLI at it. Credentials
// ride as process env (Endpoint.EnvNames), never as literals in the
// document. This is the mesh-side reimplementation of the host renderer —
// output is pinned by unit test; the host's renderer stays authoritative
// for non-codex runtimes until the full renderer suite is upstreamed.
func RenderCodexConfig(endpoints []Endpoint, byoEndpoint string) ([]byte, error) {
	var b []byte
	for _, ep := range endpoints {
		if ep.Name == "" {
			return nil, fmt.Errorf("mcp endpoint with empty name")
		}
		b = append(b, "\n[mcp_servers."+ep.Name+"]\n"...)
		switch ep.Transport {
		case "stdio":
			b = append(b, "command = \""+ep.Command+"\"\n"...)
			if len(ep.Args) > 0 {
				b = append(b, "args = ["...)
				for i, a := range ep.Args {
					if i > 0 {
						b = append(b, ", "...)
					}
					b = append(b, "\""+a+"\""...)
				}
				b = append(b, "]\n"...)
			}
			// Credential env is process-injected (EnvNames); never literal here.
		case "streamable-http":
			b = append(b, "url = \""+ep.URL+"\"\n"...)
		default:
			return nil, fmt.Errorf("mcp endpoint %q: unknown transport %q", ep.Name, ep.Transport)
		}
	}
	if byoEndpoint != "" {
		b = append(b, "\n[model_providers.ksquad-byo]\n"...)
		b = append(b, "name = \"ksquad-byo\"\n"...)
		b = append(b, "base_url = \""+byoEndpoint+"\"\n"...)
		b = append(b, "wire_api = \"chat\"\n"...)
	}
	if len(b) == 0 {
		b = []byte("# empty agentruntime MCP projection\n")
	}
	return b, nil
}
