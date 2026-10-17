# K8sAgentMesh

<p align="center">
  <img src="brand/banner.png" alt="K8sAgentMesh — the runtime-agnostic AgentMesh seam" width="720">
</p>

K8sAgentMesh is the runtime-agnostic **AgentMesh** seam: the generic mesh
surfaces of the AgentMesh architecture (approved chapter §1A/§9.3) that any
platform can host for standing AI coding agents on Kubernetes.

## What lives here

| Package | Contents |
|---|---|
| `mesh` | RuntimeAdapter 7-op contract (Spawn / Register / Route / Hibernate / Wake / Stop / Recreate), per-pod Gateway process manager, router, lifecycle state machine, child-process substrate, runtime adapters; Supervisor-side registration seams — `Registry` store interface, registration `Ingress`, TTL-prune `Sweeper`, health-sweep `SupervisorHealth` loop, route-to-endpoint selection |
| `a2a` | A2A wire types (Task / Event / Status / AgentCard / ModelRoute) |
| `apis/mesh/v1alpha1` | Mesh-owned CRD contracts — `MeshPool` (standing band, saturation, D4.2 lifecycle knobs, registration heartbeat/TTL), `MeshRoute` (selection strategy, recreate SLO, fallback), `MeshTelemetry` (per-event emission, emitter `none\|otel`, metric namespace) — plus deepcopy and CRD YAML in `apis/mesh/v1alpha1/crd/`. Types + YAML only, no controller. |

## Hosts, not owners

K8sAgentMesh is host-agnostic by design. **k8squad is host #1** (the reference
adopter): its role-based scheduling plugs in as a POLICY on the mesh scheduling
interface; CRD shapes, `coord.*` guards, telemetry spine, board/SCM and memory
stay host-side behind the seam. A second adopter maps the same elements onto
its own equivalents.

## CRD posture (P3.8)

The module owns a **thin, host-agnostic CRD set** — only for the concerns mesh
code itself consumes. A CRD is mesh-owned iff mesh code consumes it AND it is
host-reference-free AND it is mechanism-not-policy:

- **Tier A (shipped here):** `MeshPool`, `MeshRoute`, `MeshTelemetry` —
  `apis/mesh/v1alpha1` types + CRD YAML, no controller. Shapes derive from the
  lifted, proven interfaces (P3.4 Registry, P3.5 SchedulingPolicy, P3.6
  LifecycleObserver), never designed ahead of them. k8squad's RolePool stays
  host #1's source of truth and *projects* into `MeshPool` semantics via the
  P3.5 seam — it is not replaced. Board decisions (ISI-5827 interaction
  `826a7541`): the `MeshTelemetry` emitter ships `none|otel` only (NATS emitter
  deferred); `MeshRoute.fallback` stays (`legacy` for host #1, `fail` for
  host #2).
- **Tier B (Go contract types, not CRDs):** the `MeshEndpoint` registration
  record behind the P3.4 `Registry` interface, and the P3.6 lifecycle-event
  schema (`mesh/EVENTS.md`) — hosts materialize these in their own store or
  CRD form.
- **Tier C (never mesh-owned):** identity CRDs (Role, AgentRuntime, RolePool,
  Team, ModelConfig, ...), scheduling policy config, the `OtelConfig` spine,
  `coord.*` custody guards, per-team rollout flags (D9.3), MemKeys.

The CRD YAML passes structural-schema validation in CI
(`hack/crd-structural-validate.sh`), which also cross-checks every spec
property against the Go types' JSON tags — extraction-clean applies to CRD
shapes exactly as to imports.

## Extraction-clean rule

No package in this module may import `github.com/K8squad/K8squad/*`. The guard
runs in CI (`hack/extraction-clean.sh`).

## Status

P3 extraction in progress — stories P3.1–P3.8 under the K8squad roadmap
tracking issue. The code is lifted, not rewritten, from the proven seam in
k8squad (P1 prototype + P2 second-adapter proof: zero control-plane change).

## License

Apache-2.0 — see [LICENSE](LICENSE).
