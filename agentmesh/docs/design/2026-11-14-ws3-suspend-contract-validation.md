# WS3 — Substrate suspend/resume contract validation (ISI-7441)

**Spike child C of ISI-7430 productionization design (plan rev e206fd81, accepted). See ISI-7430 §4.**

Author: Winston (Architect) · Date: 2026-11-14 · Status: DONE — decision reached

---

## Question

Does the substrate ever suspend an actor **out of band** — i.e. transition a
CRD status `Running → Suspended` (ISI-5580 contract) without routing through the
gateway's `Adapter.Hibernate`? If so, the `Surcouche` suspend-flush is bypassed
and we need a CRD-status watcher. If not, no watcher is needed.

## Verdict

**Suspend is gateway-mediated. No out-of-band suspend path exists in this repo.
No CRD watcher is needed.** The decision gate's "build only if real out-of-band
suspend" condition is NOT met.

## Evidence (code, this repo)

The hypothesis (substrate has no idle-watchdog; suspend is an explicit
orchestrator call, and in this repo the gateway IS that orchestrator) is
confirmed by three independent facts.

### 1. A single `Hibernate` caller, inside the gateway sweep

```
$ grep -rn '\.Hibernate(' --include='*.go' (non-test)
mesh/gateway.go:420:   err := g.cfg.Adapter.Hibernate(ctx, leg.h)   // sweep()
agentmesh/surcouche.go:130: return s.substrate.Hibernate(ctx, h)    // decorator passthrough
```

There is exactly **one** site that initiates a freeze: the lifecycle sweep
(`gateway.go:347 sweep` → `:420`). The surcouche line is the decorator
forwarding to the substrate *after* its flush — not an independent caller.

### 2. Every `Wake` caller is inside the gateway too

```
mesh/gateway.go:441   sweep()  — wake-before-stop (graceful SIGTERM)
mesh/gateway.go:836   Route    — SIGCONT before task delivery
mesh/gateway.go:1087  Stop     — graceful pod shutdown
```

All resume actuation is `g.cfg.Adapter.Wake`. Because the production adapter IS
the `Surcouche` (it satisfies `mesh.RuntimeAdapter`, `surcouche.go:96`), resume
interception is synchronous and total — the gateway cannot resume an actor
without the surcouche's reground running first.

### 3. The gateway owns the clock; the control plane is downstream

`mesh/lifecycle.go` (LifecycleConfig doc): *"The GATEWAY owns the clock — it
runs the CheckInterval sweep over its own processes; the Supervisor only reacts
to the resulting state transitions, keeping the control plane off the
per-process hot path."*

`mesh/scheduling.go:131` (PoolStatusWriter): the CRD status subresource is a
**reporting sink** the host writes INTO — *"The host owns the sink (a CRD status
subresource, a metrics label set); the report shape is the scheduler's."* It is
an output of actuation, never an input that triggers it.

The only lifecycle-related CRD is `RolePool` (ISI-5824), which carries declarative
lifecycle **knobs** (`spec.lifecycle`: checkInterval / idleToHibernate /
hibernateToStop / alwaysUp) the gateway *reads* — config, not a suspend event.

### 4. No controller/reconcile/finalizer/webhook/preStop touches suspend

A sweep for `controller | watch | Suspended | reconcile | finalizer | preStop |
webhook` over `mesh/` + `agentmesh/` finds only: the pool-count `Scheduler`
reconcile (provisioning target-N, not freeze) and forward-looking CRD comments.
Nothing actuates `Hibernate` from a k8s control loop.

## Why a watcher stays unnecessary even under an out-of-band freeze

This is the load-bearing safety argument, independent of the code topology
above — it is why the decision gate is correctly conservative.

WS1 write-through (GrailStore + WAL) makes an out-of-band freeze **safe by
construction**. Suppose k8s froze or evicted a pod without routing through
`Adapter.Hibernate` (e.g. node pressure eviction, a CRI pause, an external
controller). The actor loses *at most its uncommitted working tier* — which is
**exactly what a crash loses**, a case WS1 already covers — and
resume-reground reads the last flushed durable state. No new data-loss class is
introduced.

A CRD-status event path would therefore buy only **proactive flush** (seal the
working tier *before* an anticipated freeze instead of relying on write-through
durability). That is a deferrable optimization, not a correctness requirement.

## Rejected alternatives (layering)

- **Pre-suspend hooks** (k8s finalizer / validating webhook / container
  `preStop`): rejected. They couple the substrate to the memory layer and
  invert the AgentMesh layering (substrate must stay unaware memory exists —
  `surcouche.go:52` doc). A `preStop` also only fires on pod *termination*, not
  on a SIGSTOP-style freeze, so it would not even cover the stated case.
- **Speculative CRD watcher now**: rejected per the decision gate — no evidence
  of an out-of-band actuator, and WS1 removes the correctness motivation.

## Boundary condition for a future revisit (honest caveat)

The finding is scoped to **this repo's deployment model**, where the gateway is
the suspend orchestrator. If a future substrate implementation actuates suspend
via a k8s controller that drives `Running → Suspended` through CRI pause / a
reconcile loop **without** calling `Adapter.Hibernate`, that would be the real
out-of-band path this gate guards against. Even then WS1 keeps it safe, so the
watcher would remain a proactive-flush optimization — file it as such if/when
that substrate variant ships, referencing this note.

## Decision

Close ISI-7441 **done — no CRD watcher needed**. No follow-up impl child filed.
WS3 of the ISI-7430 design is validated as "no speculative CRD watcher".
