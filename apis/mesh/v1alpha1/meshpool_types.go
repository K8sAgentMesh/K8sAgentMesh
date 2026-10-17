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

// OverflowPolicy is the saturation behavior when every standing process is
// busy (chapter §5.3 tiers, lifted into mesh.GatewayConfig.Max).
const (
	// OverflowPolicyGrow: grow standing processes toward standing.max
	// (each grown slot kept until the idle shrink reclaims it), then serve
	// the burst with transient cold-overflow processes torn down when the
	// task settles.
	OverflowPolicyGrow = "grow"
	// OverflowPolicyReject: refuse with ErrNoIdleProcess — the Max<=Min
	// shape exactly; no overflow (additive-only, D9.3).
	OverflowPolicyReject = "reject"
)

// MeshPool defaults — the operator defaults behind the P3.5/P3.3 mechanism
// (mesh.DefaultCheckInterval/DefaultIdleToHibernate/DefaultHibernateToStop,
// mesh.DefaultGrowIdleShrink, the gateway's 30s registration beat and the
// registry's 90s routability TTL). A host controller stamps these onto the
// gateway when the spec leaves them unset; the mechanism still owns the
// zero-value behavior.
const (
	// DefaultCheckInterval: gateway state-sweep cadence (D4.2).
	DefaultCheckInterval = Duration("30s")
	// DefaultIdleToHibernate: idle bound before hibernation (D4.2, S4 only).
	DefaultIdleToHibernate = Duration("5m")
	// DefaultHibernateToStop: idle/hibernate bound before stop (D4.2) — the
	// RAM reclaim; recreate-from-envelope is the way back.
	DefaultHibernateToStop = Duration("4h")
	// DefaultGrowIdleShrink: how long a saturation-grown slot may stand
	// idle before the sweep reclaims it (§5.3 band decay).
	DefaultGrowIdleShrink = Duration("2m")
	// DefaultHeartbeatInterval: gateway registration ticker — each tick
	// re-POSTs the full endpoint set, the same loop IS the heartbeat.
	DefaultHeartbeatInterval = Duration("30s")
	// DefaultEndpointTTL: endpoint routability TTL — three missed 30s
	// heartbeats (P3.4: a config parameter, never a host constant).
	DefaultEndpointTTL = Duration("90s")
)

// MeshPool is the generic standing-process pool: the host-agnostic core of
// what k8squad's RolePool declares, minus every host reference. Every field
// is a real, consumed knob of the lifted mechanism (mesh.PoolPlan /
// mesh.GatewayConfig / mesh.LifecycleConfig) — never a projection of host
// identity. Host #1 (k8squad) keeps RolePool as the source of truth and
// PROJECTS into these semantics through the P3.5 SchedulingPolicy seam;
// MeshPool is the host-#2 fallback and the wire-honest description of what
// a gateway pod consumes.
type MeshPool struct {
	TypeMeta `json:",inline"`
	// ObjectMeta carries the object's identity. The namespace is the
	// isolation boundary the pool serves (host #1: the team namespace);
	// poolId itself stays opaque.
	ObjectMeta `json:"metadata,omitempty"`
	Spec       MeshPoolSpec   `json:"spec,omitempty"`
	Status     MeshPoolStatus `json:"status,omitempty"`
}

// MeshPoolList is the list form of MeshPool.
type MeshPoolList struct {
	TypeMeta `json:",inline"`
	ListMeta `json:"metadata,omitempty"`
	Items    []MeshPool `json:"items"`
}

// MeshPoolID is the opaque identity pair a pool serves: the (team, role)
// dispatch lookup key (chapter §5.2). It is host-agnostic — a host maps its
// own identity objects onto these strings through its scheduling policy;
// no RoleRef, no TeamRef, no AgentRuntimeRef belongs here (seam test #2).
type MeshPoolID struct {
	// Team is the isolation boundary the pool serves.
	Team string `json:"team"`
	// Role is the role the pool's processes stand for.
	Role string `json:"role"`
}

// MeshEnvelope is the inline generic envelope (the module's mesh.Envelope
// on the wire): the resolved standing identity — runtime type, model,
// skills — a process is pre-loaded with BEFORE any task arrives. Resolved
// values, never host references: the scheduling policy resolves Role/
// AgentRuntime/ModelConfig into these fields, exactly as P3.5's PoolPlan
// carries an already-resolved Envelope.
type MeshEnvelope struct {
	// RuntimeType is the coding-agent flavor to spawn ("opencode",
	// "codex" — the module's RuntimeAdapter set).
	RuntimeType string `json:"runtimeType"`
	// Model is the resolved model id the process serves with. Empty uses
	// the runtime's default.
	Model string `json:"model,omitempty"`
	// ModelRoute is the resolved BYO OpenAI-compatible provider seam; empty
	// keeps the runtime's fixed vendor wire.
	ModelRoute *MeshModelRoute `json:"modelRoute,omitempty"`
	// Skills is the announced skill set surfaced on the agent card.
	Skills []string `json:"skills,omitempty"`
	// Identity is the standing agent identity stamped on the agent card
	// and the process's telemetry.
	Identity *MeshAgentIdentity `json:"identity,omitempty"`
	// WorkDir is the sandbox working directory the runtime runs in; empty
	// uses the host's default.
	WorkDir string `json:"workDir,omitempty"`
	// Env is extra process environment the host layers onto the launched
	// runtime (e.g. a resolved credential reference). Values may be secret
	// material and MUST NOT be logged.
	Env map[string]string `json:"env,omitempty"`
	// Source is the per-process source-checkout layout (write discipline):
	// when set, the process runs on source.branch in source.worktreeDir —
	// its exclusive write surface — with source.teamMainDir read-only.
	// nil keeps the plain WorkDir shape (no source isolation).
	Source *MeshSourceSpec `json:"source,omitempty"`
}

// MeshModelRoute is the resolved model-provider seam: an OpenAI-compatible
// base URL, the model id served there, and an optional bearer. Resolved
// values, not host refs (a host resolves its ModelConfig CR into this).
type MeshModelRoute struct {
	// Endpoint is the OpenAI-compatible base URL (e.g. http://ollama:11434/v1).
	Endpoint string `json:"endpoint,omitempty"`
	// Model is the model id/name served at Endpoint.
	Model string `json:"model,omitempty"`
	// Token is an optional bearer for the endpoint. Secret material:
	// MUST NOT be logged; hosts that can should inject via env/secret
	// instead of the object.
	Token string `json:"token,omitempty"`
}

// MeshAgentIdentity is the standing agent identity (name/squad/project).
type MeshAgentIdentity struct {
	Name    string `json:"name"`
	Squad   string `json:"squad,omitempty"`
	Project string `json:"project,omitempty"`
}

// MeshSourceSpec is the per-process source-checkout layout.
type MeshSourceSpec struct {
	// TeamMainDir is the shared team-main checkout (read-only to the
	// process).
	TeamMainDir string `json:"teamMainDir"`
	// WorktreeDir is the per-process worktree to provision and run in.
	WorktreeDir string `json:"worktreeDir"`
	// Branch is the per-process branch.
	Branch string `json:"branch"`
	// BaseRef is the ref Branch forks from; empty uses the host default.
	BaseRef string `json:"baseRef,omitempty"`
}

// StandingBand is the D2.2 standing band: min is the target-N floor the
// scheduler reconciles toward; max is the saturation ceiling. max <= min
// means no growth band (the single-floor shape, overflowPolicy: reject is
// the natural pairing).
type StandingBand struct {
	// Min is the standing process count the pool must hold. Values < 1
	// clamp to 1 at the mechanism.
	Min int32 `json:"min"`
	// Max is the saturation ceiling of the standing band. 0 or <= Min
	// keeps the band-less shape.
	Max int32 `json:"max,omitempty"`
}

// SaturationPolicy is what a gateway does when every standing process is
// busy (chapter §5.3 tiers).
type SaturationPolicy struct {
	// OverflowPolicy selects the tier set: "grow" (grow standing toward
	// max, then transient cold-overflow) or "reject" (ErrNoIdleProcess,
	// no overflow). Empty behaves as "reject".
	// +kubebuilder:validation:Enum=grow;reject
	OverflowPolicy string `json:"overflowPolicy,omitempty"`
	// GrowIdleShrink bounds how long a saturation-grown slot may stand
	// idle before the sweep reclaims it (the band's decay back toward
	// min). Empty or "0s" takes DefaultGrowIdleShrink; a negative value
	// disables grown-slot shrinking (the general hibernateToStop bound
	// still applies). Consumed only when overflowPolicy is "grow".
	GrowIdleShrink Duration `json:"growIdleShrink,omitempty"`
}

// MeshLifecycleConfig is the per-pool lifecycle knob set (chapter §6.2,
// D4.2) — the verbatim mesh.LifecycleConfig surface. The GATEWAY owns the
// clock; the supervisor only reacts to transitions.
type MeshLifecycleConfig struct {
	// CheckInterval is the gateway's regular-pace state-sweep cadence.
	// Empty takes DefaultCheckInterval.
	CheckInterval Duration `json:"checkInterval,omitempty"`
	// Hibernate arms the S4 two-tier ladder (IDLE → hibernate → STOP).
	// false (the default) keeps the REV-4 collapsed ladder — idle time
	// accrues straight to the stop bound. This is the per-pool runtime
	// kill-switch (D9.3) that lets ops disable hibernation without
	// disabling mesh routing.
	Hibernate bool `json:"hibernate,omitempty"`
	// IdleToHibernate bounds idle time before hibernation. Consumed only
	// when hibernate is armed. Empty takes DefaultIdleToHibernate.
	IdleToHibernate Duration `json:"idleToHibernate,omitempty"`
	// HibernateToStop bounds how long a process may sit idle (or
	// hibernated, at S4) with no run before the gateway terminates it —
	// the real RAM reclaim. Empty takes DefaultHibernateToStop.
	HibernateToStop Duration `json:"hibernateToStop,omitempty"`
	// AlwaysUp exempts the pool from stop (and hibernate): pinned
	// ACTIVE/IDLE, floor 1, never scaled away — the coordinator role.
	AlwaysUp bool `json:"alwaysUp,omitempty"`
}

// RegistrationConfig is the registration heartbeat/TTL pair (P3.4): the
// gateway re-POSTs the full endpoint set every heartbeatInterval and the
// registry drops an endpoint whose last beat aged past endpointTTL — TTL
// as a config parameter, never a host constant (B1).
type RegistrationConfig struct {
	// HeartbeatInterval is the registration ticker. Empty takes
	// DefaultHeartbeatInterval.
	HeartbeatInterval Duration `json:"heartbeatInterval,omitempty"`
	// EndpointTTL is the routability TTL: an endpoint whose last beat is
	// older is expired — unroutable with no dependence on the sweeper.
	// Empty takes DefaultEndpointTTL.
	EndpointTTL Duration `json:"endpointTtl,omitempty"`
}

// MeshPoolSpec is the desired standing pool.
type MeshPoolSpec struct {
	// PoolID is the opaque (team, role) identity the pool serves.
	PoolID MeshPoolID `json:"poolId"`
	// Envelope is the resolved standing envelope every process is
	// pre-loaded with.
	Envelope MeshEnvelope `json:"envelope"`
	// Standing is the target-N band (D2.2).
	Standing StandingBand `json:"standing"`
	// Saturation is the busy-everywhere behavior (§5.3). Empty behaves
	// as overflowPolicy: reject.
	Saturation *SaturationPolicy `json:"saturation,omitempty"`
	// Lifecycle is the per-pool D4.2 knob set. nil keeps the gateway's
	// operator defaults.
	Lifecycle *MeshLifecycleConfig `json:"lifecycle,omitempty"`
	// Registration is the heartbeat/TTL pair. nil keeps the defaults
	// (30s beat, 90s TTL).
	Registration *RegistrationConfig `json:"registration,omitempty"`
}

// MeshPoolStatus is the bounded status read-back (the P3.5 PoolStatusReport
// shape plus the pool pod): a count, never a per-process list.
type MeshPoolStatus struct {
	// Standing is the live standing process count (never a list).
	Standing int32 `json:"standing,omitempty"`
	// StandingKnown is false when the live-count read failed this pass —
	// the floor is still reconciled, the count is simply not fresh.
	StandingKnown bool `json:"standingKnown,omitempty"`
	// Target is the resolved target-N this pass reconciled toward.
	Target int32 `json:"target,omitempty"`
	// Pod is the gateway pod the pool currently runs in (drain/recreate
	// moves it; empty when not yet provisioned).
	Pod string `json:"pod,omitempty"`
	// ObservedGeneration is the declarative source's generation stamped on
	// the status read-back (bookkeeping).
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
