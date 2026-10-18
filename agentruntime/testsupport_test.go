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

// Test support shared by the upstreamed engine-layer tests: the host-side
// runtime-type constants the registry tests use as opaque keys (the engines
// themselves stay host-side until their own upstream pass) and the workdir
// file helpers that lived in the host's mcp_config_test.go.
const (
	RuntimeTypeHermes   = "hermes"
	RuntimeTypeOpenClaw = "openclaw"
	RuntimeTypeOpenCode = "opencode"
)

// configFiles strips the spilled envelope file from a WorkDirFile list so
// assertions see only the rendered native configs.
func configFiles(files []WorkDirFile) []WorkDirFile {
	out := make([]WorkDirFile, 0, len(files))
	for _, f := range files {
		if f.Name == envelopeFileName {
			continue
		}
		out = append(out, f)
	}
	return out
}

// hasEnvelopeFile reports whether the spilled system-context file is present.
func hasEnvelopeFile(files []WorkDirFile) bool {
	for _, f := range files {
		if f.Name == envelopeFileName {
			return true
		}
	}
	return false
}
