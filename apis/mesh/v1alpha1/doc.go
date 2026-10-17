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

// Package v1alpha1 publishes the mesh-owned CRD contracts (roadmap P3.8,
// ISI-6086; design note ISI-6063 attachment 5c0e078c; board decisions on
// ISI-5827 interaction 826a7541): the thin, host-agnostic API surface for
// the three concerns mesh code itself consumes —
//
//	MeshPool      standing min/max band, saturation/overflow policy,
//	              D4.2 lifecycle knobs, registration heartbeat/TTL
//	MeshRoute     selection strategy + recreate SLO + fallback
//	MeshTelemetry per-event emission on/off, emitter, metric namespace
//
// A CRD is mesh-owned iff mesh code consumes it AND it is
// host-reference-free AND it is mechanism-not-policy (the seam test of the
// design note). Everything failing that test stays host-side: identity CRDs
// (Role/AgentRuntime/...), scheduling policy (k8squad RolePool projects
// into MeshPool semantics via the P3.5 SchedulingPolicy seam — it is not
// replaced), the OtelConfig spine, coord guards, rollout flags, MemKeys.
// Tier B stays Go contract types, not CRDs (the MeshEndpoint record behind
// the P3.4 Registry interface; the P3.6 event schema in mesh/EVENTS.md).
//
// Shapes derive from the lifted, proven interfaces — never ahead of them:
// MeshPool from mesh.PoolPlan + mesh.GatewayConfig + mesh.LifecycleConfig
// (P3.5/P3.3), MeshRoute from the mesh route/selection leg (P3.4), and
// MeshTelemetry from mesh.LifecycleObserver's event set (P3.6). No
// controller consumes these types yet (P3.8 is types + CRD YAML only); the
// YAML is the wire-honest description of what a host controller stamps onto
// a gateway pod, and the host-#2 fallback for adopters without their own
// declarative source.
//
// The module stays dependency-free by design: TypeMeta/ObjectMeta mirror
// the metav1 identity subset locally instead of importing
// k8s.io/apimachinery. A host adopting controller-runtime binds these
// shapes onto its own generated types (or waits for the parked
// k8sagentmesh-operator follow-on); the JSON tags are the stable contract.
package v1alpha1
