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

// Package agentruntime is the standalone engine layer of the K8sAgentMesh
// project: the Runtime adapter contract, the process-wide runtime registry
// and the conformant coding-agent engines (codex, vibe).
//
// It was upstreamed from the k8squad host (pkg/shim/runtimes, ISI-6158 C3 /
// decision D1) so the mesh repo is the single source of truth for the
// engine seam; hosts consume it via third_party vendoring, recording the
// pinned mesh revision. The wire contract is preserved verbatim: the
// KSQUAD_* env var names, the ExecSpec/Progress shapes and the /task NDJSON
// semantics are the same the k8squad supervisor already serves — a vendored
// swap is a type-mapping diff, not a protocol change.
//
// Adding an engine is a new file in this package plus a Register call: zero
// core change. Engines stay declarative + pure (no I/O, no logging): the
// supervisor owns process execution.
package agentruntime
