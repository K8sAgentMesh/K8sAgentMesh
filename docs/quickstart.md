# Quickstart — vanilla K8sAgentMesh on kind

This is the end-to-end "zero host project" path: a fresh [`kind`](https://kind.sigs.k8s.io/)
cluster, a `helm install` of the vanilla chart shipped in `deploy/helm/k8sagentmesh`
(ISI-6158 C3), and a first `/task` NDJSON round-trip against the supervisor. No
k8squad chart, no k8squad values, no control-plane of any host project is involved —
that independence is the whole point of this document (plan decision D3).

> **Scope note.** The chart carries **no `dependencies:` block and no vendored
> subcharts** — `helm install` pulls nothing from any host project. You can confirm
> this yourself before installing anything: see [Prove independence](#prove-independence-offline).

## Prerequisites

| Tool | Why |
|---|---|
| `docker` (or podman) | kind runs the cluster as a container |
| `kind` ≥ 0.20 | the throwaway Kubernetes cluster |
| `helm` ≥ 3.12 | installs the vanilla chart |
| `kubectl` ≥ 1.29 | talks to the cluster (chart sets `kubeVersion: ">=1.29.0-0"`) |

You also need the four images the chart references —
`ghcr.io/k8sagentmesh/mesh-gateway`, `agentruntime`, `toolchain-vibe` and
`toolchain-codex` (tags default to the chart `appVersion`). Until those are
published to GHCR (tracked in ISI-6103), build them locally and load them
into kind:

```sh
# from the repo root — one build per images/ Dockerfile (ISI-6217)
for img in mesh-gateway agentruntime toolchain-vibe toolchain-codex; do
  docker build -f images/Dockerfile.$img -t ghcr.io/k8sagentmesh/$img:0.1.0 .
  kind load docker-image ghcr.io/k8sagentmesh/$img:0.1.0 --name mesh-qs
done
```

## 1. Create a fresh kind cluster

```sh
kind create cluster --name mesh-qs
kubectl cluster-info --context kind-mesh-qs
```

## 2. Install the vanilla chart

```sh
helm install mesh ./deploy/helm/k8sagentmesh \
  --namespace mesh --create-namespace \
  --wait
```

Confirm the release is vanilla and pulled no host-project chart:

```sh
helm list -n mesh
# NAME  NAMESPACE  REVISION  STATUS    CHART               APP VERSION
# mesh  mesh       1         deployed  k8sagentmesh-0.2.0  0.1.0

helm get all mesh -n mesh | grep -i k8squad || echo "no k8squad in release"
```

The release ships exactly: the standing-process **gateway** Deployment + Service
(the `meshgw` HTTP entrypoint launching the shipped `mesh-supervisor` per band
slot, with the pinned CLI toolchain layered in and prepended to `PATH`), the
per-task **agentruntime** supervisor Deployment + Service (the `POST /task`
ingest surface, same toolchain layering), a ServiceAccount +
ClusterRole/Binding for the mesh CRDs, and the three CRDs (`meshpools`,
`meshroutes`, `meshtelemetries`). Nothing else.

## Choosing a model backend

**The default, recommended path is BYO: point the mesh at your own
OpenAI-compatible endpoint.** Set `agentruntime.modelRoute.endpoint` to an
OpenAI-compatible `/v1` base URL and supply the API key via a Secret
(`agentruntime.credentials.existingSecret`). Left empty (the default), the
runtime stays on its native backend — Mistral for `vibe`, OpenAI for `codex`.
The chart bundles **no** model backend.

| Provider | `modelRoute.endpoint` | Credential |
|---|---|---|
| **OpenAI** | `https://api.openai.com/v1` | `OPENAI_API_KEY` via `existingSecret` |
| **Anthropic** | an OpenAI-compatible proxy, e.g. LiteLLM: `http://litellm.llm.svc:4000/v1` | key held by the proxy — the mesh speaks only the OpenAI wire, so the proxy hop is required |
| **Ollama (external / self-hosted)** | `http://<your-ollama-host>:11434/v1` | none |
| **Ollama (in-cluster)** | `http://ollama.ollama.svc:11434/v1` | none — **optional local demo only; slow, not for production** |

For example, to install against OpenAI:

```sh
helm install mesh ./deploy/helm/k8sagentmesh \
  --namespace mesh --create-namespace --wait \
  --set agentruntime.modelRoute.endpoint=https://api.openai.com/v1 \
  --set agentruntime.modelRoute.model=gpt-4o-mini \
  --set agentruntime.credentials.existingSecret=my-openai-creds
```

In-cluster Ollama is strictly opt-in: the chart ships none, and you stand one
up yourself only for a local demo. (The CI `live-validation` lane runs an
in-cluster Ollama as a zero-credential test convenience — never the product
default; see `test/live/README.md`.)

## 3. First `/task` NDJSON round-trip

The supervisor serves the A2A `/task` contract as a newline-delimited JSON
stream: one `a2a.Event` per line, the stream closing exactly once when the
task reaches a **terminal** state. Port-forward the gateway Service (routes
through the standing band) — or the agentruntime Service (direct ingest) —
and post a task:

```sh
kubectl -n mesh port-forward svc/k8sagentmesh-gateway 8080:8080 &

curl -N -s \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/x-ndjson' \
  -d '{"a2a_task_id":"qs-1","envelope":{"input":"say hello"}}' \
  http://127.0.0.1:8080/task
```

Each line is an `a2a.Event` (`status`/`message`/`tool`), the first a
`working` status. The **settle** contract: read until a line whose
`payload.state` is one of `completed`, `failed`, or `canceled` — these are
the terminal states (`a2a.TaskState.IsTerminal()`), and the stream closes
right after. A well-behaved client treats stream-close without a terminal
line as a transport failure, not success.

```jsonc
{"seq":1,"a2a_task_id":"qs-1","type":"status","payload":{"state":"working"}}
{"seq":2,"a2a_task_id":"qs-1","type":"status","payload":{"state":"completed"}}   // <- terminal; stream closes here
```

## Prove independence (offline)

You don't need a cluster to prove the chart has zero host-project coupling:

```sh
helm lint deploy/helm/k8sagentmesh          # 0 failures
helm template vanilla deploy/helm/k8sagentmesh | grep -i k8squad   # (no matches)
grep -n 'dependencies' deploy/helm/k8sagentmesh/Chart.yaml         # (no matches)
ls deploy/helm/k8sagentmesh/charts 2>/dev/null                     # (no such dir)
```

No `dependencies:`, no `charts/` subdir, no `Chart.lock` — `helm install`
resolves nothing beyond this one chart.

## Tear down

```sh
helm uninstall mesh -n mesh
kind delete cluster --name mesh-qs
```
