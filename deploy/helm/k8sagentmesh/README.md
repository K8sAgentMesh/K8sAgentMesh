# k8sagentmesh (vanilla chart)

Vanilla K8sAgentMesh install (ISI-6158 C3): the standing-process gateway,
the per-task agentruntime supervisor, RBAC and the mesh CRDs. No host
project required — images come from `ghcr.io/K8sAgentMesh/*`, built by the
tag-driven release workflow.

- **Chart version = appVersion = repo tag** (`vX.Y.Z`): the release
  workflow stamps both and attaches the packaged chart to the GitHub
  release. Versioning is independent of any host project (plan decision D3).
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
