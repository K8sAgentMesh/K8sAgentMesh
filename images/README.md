# images/ — the vanilla substrate images (ISI-6217)

Four images, each independently versioned and released by the tag-driven
workflow (`.github/workflows/release.yml`), all build-gated on every PR by
the CI `images` job (`.github/workflows/ci.yml` — an unbuildable pin can
never land silently again):

| Image | Dockerfile | Role |
|---|---|---|
| `mesh-gateway` | `Dockerfile.mesh-gateway` | The gateway pod: `meshgw` (HTTP `/healthz` `/ready` `/task` entrypoint) + `mesh-supervisor` (the standing band slot binary) + `mesh-gateway` (the minimal no-HTTP variant). debian-slim base — the pod hosts the band, whose children exec the layered toolchain CLIs. |
| `agentruntime` | `Dockerfile.agentruntime` | The per-task supervisor pod: `mesh-supervisor` (HTTP ingest — what the chart's Deployment runs) + `agentruntime` (stdio supervisor for one-shot lanes). |
| `toolchain-vibe` | `Dockerfile.toolchain-vibe` | The pinned uv/Python substrate: `mistral-vibe==2.9.6` (the engine's `py-v2.9.6` pin; the former `vibe==0.3.0` never resolved on PyPI — the CLI ships as `mistral-vibe`). |
| `toolchain-codex` | `Dockerfile.toolchain-codex` | The pinned Node substrate: `@openai/codex@0.152.0` (the engine's `rust-v0.152.0` pin) + the vendored `mesh-codex-exec` D4 wrapper. |

## Toolchain layering (chart wiring)

The supervisor images stay small; the pinned CLI substrate is layered on
by the chart's initContainer (`gateway.toolchain` /
`agentruntime.toolchain`): the toolchain image copies its CLI tree onto an
emptyDir (`/toolchain/.local` for the uv install, `/toolchain/usr-local`
for the npm one), and the Deployment prepends both to `PATH`
(ISI-6217 gap 4 — without the amendment the spawned `vibe` /
`mesh-codex-exec` binaries would not resolve).

Repository defaults by runtime flavor: `runtimeType: vibe` →
`toolchain-vibe`, `runtimeType: codex` → `toolchain-codex` (override with
`*.toolchain.image.repository`).

## The D4 codex wrapper

`mesh-codex-exec` (installed by `Dockerfile.toolchain-codex`) is the
vendored stdin wrapper the agentruntime codex engine execs: it combines the
spilled system-context file (`KSQUAD_SYSTEM_CONTEXT_FILE`) with
`KSQUAD_INPUT` and pipes the envelope into `codex exec -`, argv untouched.
Hosts embedding the engine with their own wrapper (the k8squad host's
`ksquad-codex-exec`) keep their name via `MESH_CODEX_EXEC_BIN`.
