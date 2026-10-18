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

// Runtime type constants (the mesh-side source of truth; the k8squad host
// historically kept these in its api/v1alpha1 — D1 moves them here).
const (
	// RuntimeTypeCodex is OpenAI's official Rust coding agent.
	RuntimeTypeCodex = "codex"
	// RuntimeTypeVibe is Mistral's headless `vibe` coding agent.
	RuntimeTypeVibe = "vibe"
	// RuntimeTypeGoose is the goose coding agent (aaif-goose/goose, pinned
	// v1.55.0 by the ISI-6128 Phase A spike; mesh half =
	// mesh.NewGooseAdapter, ISI-6197).
	RuntimeTypeGoose = "goose"
)

// Credential types advertised on the Agent Card (a2a.AuthInfo.Type): the
// native secret shape each engine maps the generic per-user credential to.
const (
	// CredentialTypeOpenAI is the codex lane (OPENAI_API_KEY).
	// #nosec G101 -- a credential-TYPE label for the Agent Card, not a secret.
	CredentialTypeOpenAI = "openai_api_key"
	// CredentialTypeMistral is the vibe lane (MISTRAL_API_KEY).
	CredentialTypeMistral = "mistral_api_key"
)

// Package mcp is the minimal MCP-intermediate-representation seam the
// engine layer needs (upstreamed from the k8squad host pkg/capability IR,
// The full renderer suite stays host-side until it is upstreamed in its own
// pass; the struct here is field- and tag-identical to the host IR so the
// vendoring seam is a type-mapping diff, not a re-design. Secret material
// never lives in this structure — only NAMES and refs (ADR-045 D5).

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
