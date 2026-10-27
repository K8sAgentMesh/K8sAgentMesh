# CrewAI Integration (P4.3)

**Status:** module half shipped (adapter #5 + wire router). Design: ISI-6129
(accepted, plan rev 10). Implementation story: ISI-6547.
**Pinned stack:** `crewai[a2a]==1.15.28` → `a2a-sdk~=0.3.10` (Google A2A
protocol **0.3.0**), Python ≥3.10,<3.14. Verified 2026-10-23 against
PyPI, `crewAIInc/crewAI` `lib/crewai/src/crewai/a2a/config.py` (main), and
a2a-protocol.org.

## Posture (Decision A)

CrewAI lands as **adapter #5**: `NewCrewAdapter` (`mesh/crewai.go`) over the
shared `ChildProcessAdapter` base. The standing child process is the crew's
own A2A server — not a coding CLI — spawned on the host substrate with the
crewai flavor stamped into its env. The pool is provisioned **always-on**
(`LifecycleConfig{AlwaysUp: true, Hibernate: false}`, host-side wiring);
`Hibernate`/`Wake` are overridden as telemetry-emitting no-ops
(`hibernate.noop` / `wake.noop`) so no SIGSTOP can ever land on an asyncio
server that may hold open SSE connections — the cooperative override
`childproc.go` anticipated, safe because D9.1 only ever hibernates IDLE
processes. Direct registry registration was rejected (protocol-unaware route
leg = control-plane change); revisit when a second foreign A2A service wants
to join.

## Wire (Decision B) — spike findings

The whole Google A2A wire lives inside `crewaiRouter` (`mesh/crewaiwire.go`),
injected as the adapter's **default Router** (R11: wire churn is absorbed at
the adapter seam). The mesh keeps speaking only the module wire.

### Transport decision: JSON-RPC

CrewAI's `ServerTransportConfig` ships `jsonrpc` on a **default factory**
(`rpc_path` `/a2a`, card at `/.well-known/agent-card.json`) while `grpc` and
`http_json` default to `None`. JSON-RPC is therefore the **only binding a
stock crew is guaranteed to serve** — the design's HTTP+JSON preference loses
to guaranteed presence. HTTP+JSON remains a future `WithRouter` swap if the
host registry ever pins a crew configured with it.

### Wire map (module ↔ Google A2A 0.3.x)

| Module op | CrewAI side | Notes |
|---|---|---|
| V1 SubmitTask | `message/stream` (JSON-RPC POST `/a2a`) | One send, one stream: the SSE response **is** the submission and the event stream. `message/send` is only the non-streaming fallback (`-32004`). |
| V2 StreamEvents | SSE `data:` frames of JSON-RPC envelopes | `result` frames classified 0.3.x `kind` (`task` / `status-update` / `artifact-update` / `message`) **and** 1.0 wrapper shapes (`{"statusUpdate": …}`). |
| V3 GetStatus | `tasks/get` | The reattach path (below). |
| V4 CancelTask | `tasks/cancel` | Issued best-effort by `Stream.Close()` on a still-live task — the seam's equivalent of the HTTP router's cancel-on-transport-tear. |
| V6 GetAgentCard | `GET /.well-known/agent-card.json` | Informational only; the registry stores cards opaquely. |

### Event mapping

| Google frame | Module event | Notes |
|---|---|---|
| Task snapshot | `EventStatus` | ids adopted (`googleID`, `contextId`); snapshot artifacts surface too |
| `status-update` | `EventStatus` (+`EventMessage` when `status.message` carries text) | reason lifted via `a2a.ToolErrorSummary` (bounded, NFR-2) |
| `artifact-update` | `EventArtifactRef` + `EventMessage` | ref points at `a2a://<googleTaskId>/<artifactId>`, `sha256` stays **empty** (never fabricated — inline Google artifacts have no coord-committed hash); the text content rides the accompanying message so nothing is lost |
| `message` | `EventMessage` | F16: `trust: "untrusted"`, display only |
| — | `EventTool` | **never emitted**: the Google wire has no tool-streaming surface; the card advertises none |

### State mapping

`submitted`/`working`/`input-required`/`completed`/`failed`/`canceled`/
`auth-required` map 1:1 (normalizer accepts 0.3.x slugs, `task-state/`
prefixes, and 1.0 `TASK_STATE_*` ProtoJSON forms). `rejected` → `failed`
(reason preserved). Unknown states → `working` with an `unmapped crew state`
reason; unknown/empty → `submitted`. The 0.3.x `final` flag settles the
stream even on an unrecognized terminal state name.

### C1 idempotency (the one real gap)

The module contract says a re-route on `a2a_task_id` must **reattach, never
double-execute**. CrewAI documents **no task-id dedup** (task ids are
server-generated; `messageId` dedup is a spec MAY; a message carrying an
existing `taskId` is *multi-turn* semantics — it would trigger a second crew
run). Closure is adapter-owned, in three layers:

1. **In-flight map** `a2a_task_id → crewTaskState` (ids + seq cursor + seen
   artifacts). A re-route while live reattaches via `tasks/get` polling
   (0.3.x has no `tasks/subscribe`) — never a second message.
2. **Completed map** (bounded, 1024): a re-route on a task the router saw
   terminal replays the terminal status with no crew round-trip.
3. **Torn SSE tail** on a live task hands over to the same poll pump, so the
   follower keeps a gap-free sequence (C4; the seq cursor lives on the shared
   task state, continuing across reattach).

Cross-restart dedup is best-effort (router-lifetime memory), matching the
shim engines' process-local dedup — recorded as a known limitation.

### Auth and model route

- Server auth defaults to `SimpleTokenAuth` reading **`AUTH_TOKEN`** from the
  process env: the host resolves the secret into the envelope env (pass-through
  in `crewLaunch`) and wires the same value as the router's Bearer token via
  `NewCrewaiRouter(client, token)`.
- A BYO OpenAI-compatible `ModelRoute` rides the litellm lane
  (`OPENAI_BASE_URL` / `OPENAI_API_KEY`, tokenless lanes get the `ollama`
  placeholder — the goose treatment). Crew model ids stay crew-author-side:
  the crew project owns its agents and their models in code; `Envelope.Model`
  overrides only the card's model id.
- The launch env is deterministic (sorted) — byte-identical rebuilds for
  `Recreate`. The crew project dir is the envelope `WorkDir`; `Envelope.Source`
  overrides it with the per-process worktree like every other adapter.

## Honest capabilities (initial card)

Streaming ✓ (SSE frames) · BYO model endpoint ✓ (litellm lane) ·
interactivePrompt ✗ · toolCalls ✗ (no module `EventTool` surface on this
wire) · non-coding: `Envelope.Source` stays nil unless a crew writes files.

## Zero control-plane diff (the P2/P4.x acceptance property)

Adapter #5 lands as **new files only** (`mesh/crewai.go`, `mesh/crewaiwire.go`
+ tests). Gateway, route selection, lifecycle, supervisor, scheduling and
ingress are untouched — the fifth runtime (and the first non-CLI,
framework-class one) exercises the seam with zero control-plane change,
which is the P4.3 generalization proof.

## Live lane (phase 4)

`test/live/crewai/` drives the real pinned stack end-to-end: a real Crew
kickoff (litellm lane → a zero-credential OpenAI-compatible stand-in) behind
the a2a-sdk JSON-RPC+SSE server, tailed by the real `crewaiRouter`. Proves
the happy path (marker-echoed output rides `EventArtifactRef`), C1 for real
(re-route on the same `a2a_task_id` while in flight reattaches — exactly one
kickoff), CancelTask (`Stream.Close()` → `tasks/cancel` server-side), the
card at the well-known path, and the `AUTH_TOKEN` bearer. The lane found and
fixed a real gap: a real a2a-sdk stream opens with a status-update, never a
Task snapshot, so id adoption now happens on event frames too (before, a
torn tail looked like "no task was ever created" and Close could never map
to `tasks/cancel`). Run: `test/live/crewai/lane.sh` (uv provisions the pinned
venv; the Go side is build-tagged `crewai_lane` and env-gated `CREWAI_LANE=1`).

## Host side (k8squad — separate story)

Runtime registry entry (Type `crewai`, credential shape `OPENAI_API_KEY` +
`AUTH_TOKEN`, pin `crewai[a2a]==1.15.28`), sandbox image gains python 3.1x +
uv + the pinned extra, RolePool wiring with the always-on lifecycle, e2e lane.
