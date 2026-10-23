# Mesh lifecycle-event contract

**P3.6** — the NATS-emission concept of the AgentMesh chapter
(§8.2/§8.3) formalized as the K8sAgentMesh module's lifecycle-event contract.
Grounding: the observability consult verdict (§11.3 a–b schema delta).

## Shape

| Role | Bearer | Notes |
|---|---|---|
| Producer | `Gateway` (`mesh/gateway.go`) | The **sole** producer. It reports through its `GatewayConfig.Observer` and never touches a telemetry SDK — the extraction contract. |
| Emitter interface | `mesh.LifecycleObserver` (`mesh/lifecycle.go`) | Host-pluggable: injected by the composition root; nil is a valid no-op. All hooks are informational — an observer error can never fail the lifecycle. |
| Gauge inputs | `mesh.ProcessStateSnapshot` + `mesh.SnapshotProcessStates` (`mesh/events.go`) | Bounded per-state counts polled by the emitter on its own cadence. |

The contract is deliberately **transport-agnostic**: events carry identity,
bounded-vocabulary fields and timings, never a telemetry-SDK type. That is what
makes a second emitter possible.

## Events

### `RouteEvent` — `ObserveRoute(ctx, e)`

One routing decision: A2A arrival → process-ready (the ≤3 s recreate-SLO
window). `ctx` carries the dispatch request's trace context so an emitter can
parent the event to the run's trace.

| Field | Type | Schema |
|---|---|---|
| `Team`, `Role`, `Pod`, `EndpointID` | `string` | Bounded routing identity (span attributes / metric label set). |
| `Selection` | `RouteSelection` | Vocabulary: `idle` \| `overflow` \| `wake`. `overflow` = recreated from the envelope for this request; `wake` = hibernated process woken (S4 only). |
| `Duration` | `time.Duration` | Arrival→ready wall-clock. |
| `Err` | `error` | Non-nil when the route failed (outcome vocabulary: `success` \| `error`). |

### `SpawnEvent` — `ObserveSpawn(ctx, e)`

One spawn or recreate with wall-clock duration. On the recreate path `ctx` is
the triggering route's context, so an emitter can nest it under the route.

| Field | Type | Schema |
|---|---|---|
| `Team`, `Role`, `Pod`, `EndpointID` | `string` | Bounded routing identity. |
| `Source` | `SpawnSource` | Vocabulary: `envelope` (recreate — the ≤3 s target path) \| `cold` (pool boot). |
| `Duration` | `time.Duration` | Spawn wall-clock. |
| `Err` | `error` | Non-nil when the spawn failed. |

### `WakeEvent` — `ObserveWake(ctx, e)`

One hibernation wake (S4): SIGCONT → ready.

| Field | Type | Schema |
|---|---|---|
| `Team`, `Role`, `Pod`, `EndpointID` | `string` | Bounded routing identity. |
| `Duration` | `time.Duration` | Wake wall-clock. Absent (zero) on a rebind wake — its cost is the respawn, already reported as a spawn with `Source=envelope`. |
| `Rebind` | `bool` | `true` when the envelope's model moved while the process slept and the wake respawned from the current envelope (X5: rebind-at-wake, never mid-stream). |
| `Err` | `error` | Non-nil when the wake failed. |

## Hibernation surface

There is deliberately **no `ObserveHibernate` hook**. The interface is lifted,
not extended (P3.3 fidelity; the reference host's emitter implements it with
zero behavior change). Hibernation signals ride the schema as-is:

- entry/exit of the hibernated tier: the `StateHibernated` gauge state
  (`active` \| `idle` \| `hibernated` \| `stopped`),
- a wake-served route: `RouteEvent{Selection=wake}` + the `WakeEvent`,
- the RAM-reclaim trade: a `SpawnEvent{Source=envelope}` recreate is the
  "had-to-recreate" (hibernate-miss) signal.

Where a full hibernate *entry* event belongs is the wire-emitter design space
(implementation #2, below): the consult's five-event NATS lifecycle set —
`register`/`deregister`/`hibernate`/`wake`/`stop` — as edge-triggered
state-change facts. `pool_saturated` stays **metric-only** (derive
`busy == max` from the gauge; a sampled threshold has no transaction to ride —
consult verdict A2).

## Process-state gauge inputs

`ProcessStateSnapshot` (`func() map[ProcState]int64`) is polled by the emitter
on its own cadence (an OTel collection cycle for implementation #1). Rules:

- cheap and non-blocking — back it with `SnapshotProcessStates(gw.Processes())`,
  the gateway's in-memory listing; **never** a live call into a process,
- stopped slots stay counted (recreate-on-demand capacity, not a leak),
- zero-count states may be omitted.

## Cardinality (NFR-OBS3 firewall)

Every label an emitter derives from this schema is bounded: identity is the
pool set `{team, role}` (+ `pod` where the host spine already carries it),
state is the fixed §6 enum, spawn source is the two-value set. Endpoint ids
and run ids are attributes only, never metric labels.

## Implementations

| # | Emitter | Status |
|---|---|---|
| 1 | k8squad `pkg/telemetry/meshobs` — OTel spine: `mesh.route`/`mesh.spawn`/`mesh.wake` spans, `ksquad.mesh.*` metrics (`a2a.ready_latency`, `spawn.latency{source}`, `wake.latency`, `hibernate.hits_total`, `hibernate.misses_total`, `processes.active` gauge) | **Live** — host-side, unchanged in behavior; implements this contract against the module interface. |
| 2 | NATS JetStream emitter | Possible later — **not in P3**. Five-event lifecycle set per the consult verdicts; reuses this event schema for identity/vocabulary. |

Adding an emitter means implementing `mesh.LifecycleObserver` (+ optionally a
gauge over `ProcessStateSnapshot`) and injecting it at the composition root —
no gateway change, no telemetry SDK in the module.

## Appendix: CrewAI wire event mapping (P4.3, adapter #5)

The crewai adapter (`mesh/crewaiwire.go`) translates Google A2A 0.3.x SSE
frames onto the module task-event schema. This appendix is the authoritative
mapping (full context: `docs/crewai-integration.md`).

| Google A2A frame | Module event | Mapping notes |
|---|---|---|
| Task snapshot (`kind: "task"`) | `EventStatus` | crew-side `googleID`/`contextId` adopted; artifacts already on the snapshot surface as updates |
| `TaskStatusUpdateEvent` (`kind: "status-update"`) | `EventStatus` | states map 1:1 (`submitted/working/input-required/completed/failed/canceled/auth-required`); `rejected` → `failed`; `status.message` text also lifts to an `EventMessage` and, when no reason exists, to the bounded status reason (`a2a.ToolErrorSummary`, NFR-2) |
| `TaskArtifactUpdateEvent` (`kind: "artifact-update"`) | `EventArtifactRef` + `EventMessage` | ref `Kind: "google-a2a-artifact"`, `URI: a2a://<googleTaskId>/<artifactId>`, `SHA256` **empty** — inline Google artifacts carry no coord-committed hash and one is never fabricated; the artifact's text parts ride the accompanying `EventMessage` so content is not lost |
| `Message` (`kind: "message"`) | `EventMessage` | F16: `trust: "untrusted"`, display only |
| *(no equivalent)* | `EventTool` | **never produced** — the Google wire has no tool-streaming surface; the crewai card advertises no tool capability |

Sequence discipline (C4): the Google wire carries no seq; the router assigns
the module seq (monotonic, gap-free from 1) on the shared per-task cursor, so
a C1 reattach (in-flight re-route → `tasks/get` polling) or a torn-SSE
handover continues the same sequence. The 0.3.x `final` flag settles the
stream even on an unrecognized terminal state name. Frame classification
tolerates both 0.3.x `kind` discriminators and 1.0 wrapper-member shapes.
