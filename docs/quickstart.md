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

You also need the two runtime images the chart references —
`ghcr.io/k8sagentmesh/mesh-gateway` and `ghcr.io/k8sagentmesh/agentruntime`
(tag defaults to the chart `appVersion`). Until those are published to GHCR
(tracked in ISI-6103), build the gateway image locally and load it into kind:

```sh
# from the repo root — builds cmd/meshgw into a distroless image (ISI-6205)
docker build -t ghcr.io/k8sagentmesh/mesh-gateway:0.1.0 .
kind load docker-image ghcr.io/k8sagentmesh/mesh-gateway:0.1.0 --name mesh-qs
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
# mesh  mesh       1         deployed  k8sagentmesh-0.1.0  0.1.0

helm get all mesh -n mesh | grep -i k8squad || echo "no k8squad in release"
```

The release ships exactly: the standing-process **gateway** Deployment + Service,
the per-task **agentruntime** supervisor Deployment, a ServiceAccount +
ClusterRole/Binding for the mesh CRDs, and the three CRDs (`meshpools`,
`meshroutes`, `meshtelemetries`). Nothing else.

## 3. First `/task` NDJSON round-trip

The supervisor serves the A2A `/task` contract as a newline-delimited JSON
stream: one JSON event per line, the stream closing exactly once when the task
reaches a **terminal** state. Port-forward the gateway Service and post a task:

```sh
kubectl -n mesh port-forward svc/k8sagentmesh-gateway 8080:8080 &

curl -N -s \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/x-ndjson' \
  -d '{"id":"qs-1","message":{"role":"user","parts":[{"text":"say hello"}]}}' \
  http://127.0.0.1:8080/task
```

Each line is a `Status` update (`submitted` → `working` → …). The **settle**
contract: read until a line whose `state` is one of `completed`, `failed`, or
`canceled` — these are the terminal states (`a2a.TaskState.IsTerminal()`), and
the stream closes right after. A well-behaved client treats stream-close
without a terminal line as a transport failure, not success.

```jsonc
{"id":"qs-1","status":{"state":"working"}}
{"id":"qs-1","status":{"state":"completed"}}   // <- terminal; stream closes here
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
