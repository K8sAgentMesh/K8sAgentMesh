# test/live/crewai — the live CrewAI wire lane (P4.3 phase 4)

The fake-A2A httptest suite (`mesh/crewaiwire_test.go`) pins the router's
semantics against a scripted server. This lane proves the same contract
against the **real pinned stack** — `crewai[a2a]==1.15.28` →
`a2a-sdk~=0.3.10` (Google A2A **0.3.0**) — with a real Crew kickoff behind
the a2a-sdk JSON-RPC+SSE server on the default binding the spike locked:
`POST /a2a` (`message/stream` SSE, `tasks/get`, `tasks/cancel`) and the agent
card at `/.well-known/agent-card.json`, bearer auth reading `AUTH_TOKEN`
(SimpleTokenAuth semantics — the contract `mesh/crewai.go` stamps).

Like everything under `test/live/`, this is **test-lane fixture code**, not
product. Nothing here is installed by the chart or shipped in any image.

## What the lane proves

1. **A live crew task routes via the mesh wire** — `NewCrewaiRouter` →
   `message/stream` → real Crew kickoff (litellm lane) → SSE frames mapped
   onto module events, settling `completed` with the crew output riding
   `EventArtifactRef`. The model answer is marker-echoed
   (`wire-probe-final-answer <marker>`) so the output provably flowed
   model → crew → A2A → router.
2. **C1 for real** — a torn SSE tail (follower closes mid-task) followed by a
   re-route on the same `a2a_task_id` reattaches via `tasks/get` polling:
   gap-free continuation seq, terminal replay, and the crew kicked off
   exactly once (`kickoffs` counter unchanged across the re-route).
3. **CancelTask maps** — `Stream.Close()` lands `tasks/cancel` server-side
   (the server-side `cancels` counter ticks).
4. **The pinned defaults hold** — card at the well-known path with
   `protocolVersion 0.3.0`; `POST /a2a` without the bearer is a 401.

## Fixture map

| File | Role |
|---|---|
| `lane.sh` | Runner: provisions an isolated uv venv with the pinned stack, runs the Go lane test |
| `lane_test.go` | The lane itself (build tag `crewai_lane`, runtime gate `CREWAI_LANE=1`) |
| `crew_server.py` | A real Crew behind the a2a-sdk JSON-RPC+SSE server; `/lane/stats` instrumentation |
| `mock_openai.py` | Zero-credential OpenAI-compatible stand-in for the litellm lane; `HOLD` marker keeps tasks in flight for the C1/cancel scenarios |

## Running

```sh
test/live/crewai/lane.sh            # provisions test/live/crewai/.venv on first run
```

Requirements: `uv`, `go`, and network access to PyPI (first run only). The
in-flight scenarios hold each task `HOLD_SECONDS` (default 6) — the whole
lane settles in well under two minutes after the venv exists.

## Host adoption (k8squad)

The lane is repo-local and self-contained: the k8squad CI can run it inside
the `Dockerfile.toolchain-crewai` image (python + uv + the pin are already
there) with `CREWAI_LANE_PYTHON=$(command -v python3)` and the Go toolchain
mounted — the module-half wire proof for the gateway `buildRuntimeAdapter`
crewai case, without a cluster.

## Known limitations

- The model is a deterministic stand-in: the lane proves the **wire**, not
  model quality (by design — spec §11/C9, no model credentials in lanes).
- The crew fixture is one agent, one task, sequential — CrewAI behaviors
  beyond kickoff→output (multi-turn, delegation, input-required) are not
  exercised; the router's input-required and failure mappings stay covered
  by the fake-A2A suite.
