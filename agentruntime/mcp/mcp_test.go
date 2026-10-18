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

package mcp

import (
	"strings"
	"testing"
)

func TestRenderCodexConfigStdio(t *testing.T) {
	out, err := RenderCodexConfig([]Endpoint{{
		Name:      "github",
		Transport: "stdio",
		Command:   "github-mcp",
		Args:      []string{"--stdio"},
		EnvNames:  []string{"KSQUAD_MCP_GITHUB_TOKEN"},
	}}, "")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"[mcp_servers.github]",
		"command = \"github-mcp\"",
		"args = [\"--stdio\"]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered config lacks %q:\n%s", want, got)
		}
	}
	// Credentials ride process env — never literals in the document.
	if strings.Contains(got, "TOKEN=") {
		t.Errorf("rendered config leaked credential material:\n%s", got)
	}
}

func TestRenderCodexConfigHTTPAndBYO(t *testing.T) {
	out, err := RenderCodexConfig([]Endpoint{{
		Name:      "search",
		Transport: "streamable-http",
		URL:       "http://search.svc/mcp",
	}}, "http://ollama:11434/v1")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := string(out)
	for _, want := range []string{
		"[mcp_servers.search]",
		"url = \"http://search.svc/mcp\"",
		"[model_providers.ksquad-byo]",
		"base_url = \"http://ollama:11434/v1\"",
		"wire_api = \"chat\"",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered config lacks %q:\n%s", want, got)
		}
	}
}

func TestRenderCodexConfigEmptyAndBad(t *testing.T) {
	out, err := RenderCodexConfig(nil, "")
	if err != nil {
		t.Fatalf("empty render: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Error("empty projection rendered to nothing; want placeholder")
	}
	if _, err := RenderCodexConfig([]Endpoint{{Name: "x", Transport: "carrier-pigeon"}}, ""); err == nil {
		t.Error("unknown transport accepted")
	}
	if _, err := RenderCodexConfig([]Endpoint{{Transport: "stdio", Command: "c"}}, ""); err == nil {
		t.Error("empty name accepted")
	}
}

func TestCredentialEnvName(t *testing.T) {
	if got := CredentialEnvName("github"); got != "KSQUAD_MCP_GITHUB_TOKEN" {
		t.Errorf("CredentialEnvName(github) = %q", got)
	}
}
