# test/live — the ISI-6211 live validation lane

Everything under `test/live/` is **test-lane fixture code**, not product: it
exists so `.github/workflows/live-validation.yml` can prove the vanilla mesh
end-to-end on a fresh kind cluster while two known substrate gaps are still
open. Nothing here is installed by the chart or shipped in any image the
chart references.

## What the lane proves

1. **Vanilla install** — the three real in-repo images (built in-job, since
   the GHCR release is not cut yet) install via the vanilla chart on a fresh
   kind cluster; `helm get all` contains zero `k8squad` references.
2. **Supervisor `/task` NDJSON settle** — one vibe task and one codex task,
   each a single NDJSON line in (the `taskRequest` wire of
   `cmd/agentruntime`), each settling per the ISI-4224 bar: a terminal
   `{"kind":"completed"|"failed"}` line **and** prompt stream close — driven
   against an in-cluster Ollama (`qwen2.5:0.5b`), the zero-credential lane
   (spec §11/C9), so no model credentials ever exist in the lane.
3. **Zero control-plane change** — cluster-wide snapshot before/after the
   whole run; the only allowed delta is the mesh release's own footprint
   (its namespace, CRDs, RBAC, workloads) and the lane's `ollama` namespace.

## Known gaps the lane documents (not works around silently)

1. **Gateway standing band needs an unshipped `shim` binary.**
   `mesh.LocalHost` execs `shim supervisor` (default `BinPath "shim"`), the
   k8squad host project's shim (`cmd/shim/supervisor.go` there). This repo
   ships no `cmd/shim` and no image carrying it, so the gateway pod's
   standing band cannot start vanilla. The lane records the rollout state
   and the `/task` HTTP probe result as evidence instead of asserting them.
2. **The agentruntime Deployment has no task-ingest surface.** The
   supervisor (`cmd/agentruntime`) reads task lines from **stdin** and emits
   NDJSON on stdout; a Deployment attaches no stdin, so the deployed pod
   EOFs and exits. The chart ships it as the substrate holder (the
   toolchain-layering initContainer proof), while actual task routing needs
   a per-task execution surface (one-shot pod / Job / service front-end).
3. **Codex needs the unshipped `ksquad-codex-exec` wrapper.**
   `agentruntime`'s codex engine execs `ksquad-codex-exec` (the k8squad D4
   stdin wrapper). This repo does not vendor it, and no codex toolchain
   image exists (plan decision D2 deferred it). The lane carries a
   test-only reimplementation (`mesh-codex-exec`) honoring the documented
   contract (read `KSQUAD_SYSTEM_CONTEXT_FILE`, combine with
   `KSQUAD_INPUT` blank-line separated — `agentruntime.EnvelopePrompt`
   semantics — and pipe the envelope into `codex exec -`), which proves the
   mesh-side wire and settle contract; vendoring a real wrapper remains
   repo work.

## Gap 5 — the engine's vibe argv targets the k8squad-internal lineage

`agentruntime`'s vibe engine passes `--auto-approve`; no public
`mistral-vibe` (checked 1.0.0–2.9.6) ever had that flag — `-p` mode
auto-approves by design there. The engine's `vibeCLIVersion` pin
(`py-v0.3.0`) references a tag that exists in no public registry, so the
argv shape is effectively k8squad-internal. The lane's fixture wraps the
real CLI with a shim that drops the flag (test-only; see
`lane-vibe.Dockerfile`). Vendoring or aligning the engine's CLI contract
with a public pin is repo work.

## Fixture map

| File | Role |
|---|---|
| `ollama.yaml` | in-cluster Ollama Deployment+Service (the zero-credential model lane) |
| `lane-vibe.Dockerfile` | supervisor + `toolchain-vibe` CLI in one image (what the chart layers via initContainer, merged for one-shot pods) |
| `lane-codex.Dockerfile` | supervisor + codex CLI + the test-only `ksquad-codex-exec` wrapper |
| `mesh-codex-exec` | the test-only D4 wrapper (see gap 3) |
| `tasks/vibe-task.json`, `tasks/codex-task.json` | one-line `taskRequest` payloads with the Ollama `modelRoute` |
| `settle-check.sh` | asserts terminal line + stream close per transcript (the ISI-4224 bar) |
| `cluster-snapshot.sh` / `cluster-diff.sh` | before/after control-plane state + allowlisted-delta assertion |
| `gateway-probe.sh` | timed `/task` HTTP probe against the gateway Service (gap-evidence, never fails the lane) |
