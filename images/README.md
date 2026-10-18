# images/ — K8sAgentMesh OCI images (ISI-6158 C3)

Tag-driven CI (`.github/workflows/release.yml`) builds and pushes these to
`ghcr.io/K8sAgentMesh/*` on `vX.Y.Z` tags. Versioning is independent of any
host project's releases (plan decision D3).

| Dockerfile | Image | Purpose |
|---|---|---|
| `Dockerfile.mesh-gateway` | `ghcr.io/K8sAgentMesh/mesh-gateway` | Standing-process gateway (`cmd/mesh-gateway`), env-configured (`MESH_RUNTIME_TYPE`, `MESH_MODEL`, `MESH_POOL_MIN/MAX`) |
| `Dockerfile.agentruntime` | `ghcr.io/K8sAgentMesh/agentruntime` | Per-task NDJSON supervisor (`cmd/agentruntime`); flavor via `KSQUAD_RUNTIME_TYPE` |
| `Dockerfile.toolchain-vibe` | `ghcr.io/K8sAgentMesh/toolchain-vibe` | Pinned uv + py3.12 + `vibe==0.3.0` substrate (the engine's `CLIVersion py-v0.3.0` pin) |

The codex toolchain image is an optional follow-up (plan decision D2).

The helm chart (`deploy/helm/k8sagentmesh`) composes these: the
agentruntime Deployment layers the toolchain CLI onto the supervisor via an
initContainer (see `values.yaml`).
