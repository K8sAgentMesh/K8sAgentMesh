# k8sagentmesh (vanilla chart)

Vanilla K8sAgentMesh install (ISI-6158 C3, substrate closed in ISI-6217):
the standing-process gateway, the per-task agentruntime ingest surface,
RBAC and the mesh CRDs. No host project required — images come from
`ghcr.io/K8sAgentMesh/*`, built by the tag-driven release workflow.

- **Chart version = appVersion = repo tag** (`vX.Y.Z`): the release
  workflow stamps both and attaches the packaged chart to the GitHub
  release. Versioning is independent of any host project (plan decision D3).
- **Gateway** (`gateway.*`): the `mesh-gateway` image runs `meshgw` — the
  HTTP entrypoint (`GET /healthz`, `GET /ready`, `POST /task` NDJSON) — and
  launches the shipped `mesh-supervisor` per standing band slot
  (`MESHGW_SHIM_BIN`, preset by the image). The band's pinned CLI substrate
  is layered onto the pod by the `gateway.toolchain` initContainer and
  prepended to `PATH` (defaults by `runtimeType`: `toolchain-vibe` /
  `toolchain-codex`).
- **agentruntime** (`agentruntime.*`): the deployed pod runs
  `mesh-supervisor` behind the `agentruntime` Service — the task-ingest
  surface (`POST /task`, one `a2a.Task` in, `a2a.Event` NDJSON out). The
  same toolchain layering applies. The stdio supervisor
  (`cmd/agentruntime`) stays shipped in the image for one-shot lanes.
- **CRDs** (`meshroutes`, `meshtelemetries`, `meshpools`) install from
  `crds/` (copied verbatim from `apis/mesh/v1alpha1/crd` at build time).
- **Credentials**: reference an existing Secret
  (`agentruntime.credentials.existingSecret`); the chart projects the key
  into the runtime-native env var (`MISTRAL_API_KEY` for vibe,
  `OPENAI_API_KEY` for codex). Raw credentials never go through values.

## Model backend

**The default path is BYO: point the mesh at your own OpenAI-compatible
endpoint.** Set `agentruntime.modelRoute.endpoint` to any OpenAI-compatible
`/v1` base URL and supply the API key through a Secret
(`agentruntime.credentials.existingSecret`). Leaving `endpoint` empty (the
default) keeps the runtime on its native default — Mistral for `vibe`
(`MISTRAL_API_KEY`), OpenAI for `codex` (`OPENAI_API_KEY`). No model backend
is bundled in the chart.

| Provider | `modelRoute.endpoint` | Credential |
|---|---|---|
| **OpenAI** | `https://api.openai.com/v1` | `OPENAI_API_KEY` via `existingSecret` |
| **Anthropic** | an OpenAI-compatible proxy, e.g. LiteLLM: `http://litellm.llm.svc:4000/v1` | key held by the proxy (the mesh speaks only the OpenAI wire — the proxy hop is required) |
| **Ollama (external / self-hosted)** | `http://<your-ollama-host>:11434/v1` | none |
| **Ollama (in-cluster)** | `http://ollama.ollama.svc:11434/v1` | none — **optional local demo only; slow, not for production** |

The chart ships **no** in-cluster Ollama; bringing one up is an explicit,
opt-in choice for local demos. (The CI `live-validation` lane does run an
in-cluster Ollama, but that is a test-only zero-credential convenience — see
`test/live/README.md` — never the product default.)

## Quick start

```sh
# BYO OpenAI-compatible endpoint (recommended):
helm install mesh deploy/helm/k8sagentmesh \
  --set agentruntime.modelRoute.endpoint=https://api.openai.com/v1 \
  --set agentruntime.modelRoute.model=gpt-4o-mini \
  --set agentruntime.credentials.existingSecret=my-openai-creds

# …or stay on the runtime's native default (vibe → Mistral):
helm install mesh deploy/helm/k8sagentmesh \
  --set agentruntime.credentials.existingSecret=my-mistral-creds
```

For a full end-to-end validation (vibe + codex settle via /task NDJSON on
a vanilla kind install) see ISI-6204.

## Task ingest

```sh
kubectl -n <ns> port-forward svc/<release>-agentruntime 8080:8080
curl -N -s -H 'Content-Type: application/json' \
  -d '{"a2a_task_id":"t-1","envelope":{"input":"say hello"}}' \
  http://127.0.0.1:8080/task
```

The stream settles per the ISI-4224 bar: a terminal
`{"type":"status","payload":{"state":"completed"|"failed"}}` line, then the
body closes. Routing through the gateway's standing band instead
(`svc/<release>-gateway`) uses the identical wire with `?target=` selection.
