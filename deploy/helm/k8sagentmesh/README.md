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

## Quick start

```sh
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
