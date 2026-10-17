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

package v1alpha1

// TelemetryEmitter is the sink selector for mesh lifecycle events
// (mesh.LifecycleObserver implementations). Per the board decision on
// ISI-5827 interaction 826a7541 the CRD ships none|otel only — the NATS
// emitter is deferred until the P3.6 event schema proves out, at which
// point "nats" joins the vocabulary without any other shape change.
const (
	EmitterNone = "none"
	EmitterOTel = "otel"
)

// TelemetryEvent is the per-event on/off vocabulary — exactly the
// LifecycleObserver hook set formalized by P3.6 (mesh/EVENTS.md). There is
// deliberately no "hibernate" event: the interface has no ObserveHibernate
// hook (lift-not-extend, P3.3 fidelity); hibernation rides the
// process-state gauge (active|idle|hibernated|stopped), and a
// SpawnEvent{Source=envelope} is the hibernate-miss signal.
const (
	TelemetryEventRoute = "route"
	TelemetryEventSpawn = "spawn"
	TelemetryEventWake  = "wake"
)

// MeshTelemetry is the observability object: the declarative on/off + sink
// selector over the P3.6 lifecycle-event contract. It does NOT duplicate a
// host's OtelConfig (exporter endpoints, sampling — the OTel spine stays
// host-side); it references the spine's existence, not its fields.
type MeshTelemetry struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`
	Spec       MeshTelemetrySpec   `json:"spec,omitempty"`
	Status     MeshTelemetryStatus `json:"status,omitempty"`
}

// MeshTelemetryList is the list form of MeshTelemetry.
type MeshTelemetryList struct {
	TypeMeta `json:",inline"`
	ListMeta `json:"metadata,omitempty"`
	Items    []MeshTelemetry `json:"items"`
}

// MeshTelemetryScope scopes one MeshTelemetry object. Empty team/role means
// cluster-wide — the default emission posture.
type MeshTelemetryScope struct {
	// Team scopes emission to one team's pools.
	Team string `json:"team,omitempty"`
	// Role scopes emission to one role. Empty matches every role of the
	// team.
	Role string `json:"role,omitempty"`
}

// OTelToggles is the spans/metrics pair for emitter: otel. It toggles only
// emission channels only — the exporter spine (endpoints, sampling,
// OtelConfig) is the host's.
type OTelToggles struct {
	// Spans turns the mesh.route/mesh.spawn/mesh.wake spans on/off.
	// nil behaves as true.
	Spans *bool `json:"spans,omitempty"`
	// Metrics turns the namespace.* metric set on/off. nil behaves as
	// true.
	Metrics *bool `json:"metrics,omitempty"`
}

// MeshTelemetrySpec is the desired emission posture.
type MeshTelemetrySpec struct {
	// Scope scopes the object; absent = cluster-wide.
	Scope *MeshTelemetryScope `json:"scope,omitempty"`
	// Emitter is the sink: "none" (no observer wired — events are dropped)
	// or "otel" (the host's OTel spine impl, e.g. k8squad meshobs).
	// Empty behaves as "none". "nats" is reserved for the deferred
	// implementation #2 (board decision 826a7541) and is rejected by the
	// schema until it ships.
	// +kubebuilder:validation:Enum=none;otel
	Emitter string `json:"emitter,omitempty"`
	// Namespace is the metric prefix (today's host impl: "ksquad" →
	// ksquad.mesh.*). Empty uses the emitter's default.
	Namespace string `json:"namespace,omitempty"`
	// Events is the per-event on/off subset of the P3.6 hook set
	// (route|spawn|wake). Empty means ALL events on; listing a subset
	// turns the unlisted hooks off.
	// +kubebuilder:validation:Enum=route;spawn;wake
	// +kubebuilder:validation:MaxItems=3
	Events []string `json:"events,omitempty"`
	// OTel toggles the OTel emission channels. Consumed only when
	// emitter is "otel". nil keeps both channels on.
	OTel *OTelToggles `json:"otel,omitempty"`
}

// MeshTelemetryStatus is the bounded emission read-back.
type MeshTelemetryStatus struct {
	// EmittedTotal is the count of emitted events across the enabled
	// hook set.
	EmittedTotal int64 `json:"emittedTotal,omitempty"`
	// LastEmitAt is the RFC 3339 timestamp of the most recent emit.
	LastEmitAt string `json:"lastEmitAt,omitempty"`
}
