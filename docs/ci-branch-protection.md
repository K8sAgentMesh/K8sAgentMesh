# Branch-Protection Checklist — K8sAgentMesh main branch (Phase 1 CI)

Deliverable for ISI-6102 (validation task from the approved CI plan on ISI-6095, plan revision 82afbd79).
FINAL job IDs confirmed against the landed `.github/workflows/ci.yml` (commit 59f27a3, ISI-6101), actionlint 1.7.7 clean.
This text is to be pasted into the live test PR description for the repo admin to apply.

## Where to apply

GitHub → `K8sAgentMesh/K8sAgentMesh` → Settings → Branches → Branch protection rules →
Add rule for `main`.

## Required status checks ("Require status checks to pass before merging")

Enter these exact check names (GitHub check runs use the job IDs from ci.yml):

| # | Check name (job ID) | CI job name (as shown) | What it enforces |
|---|--------------------|------------------------|------------------|
| 1 | `build` | Build + vet | `go build ./...` + `go vet ./...` |
| 2 | `unit` | Unit tests (race detector) | unit tests pass, race-detector clean (`-race -count=1`) |
| 3 | `quality` | Lint + gofmt + module hygiene | golangci-lint (config `.golangci.yml`), gofmt, `go mod tidy -diff` zero-dep guard |
| 4 | `guards` | Extraction-clean + CRD structural guards | `hack/extraction-clean.sh` + `hack/crd-structural-validate.sh` |
| 5 | `coverage` | Coverage (soft gate: warn <70%, fail <50%) | tests with `-coverprofile`, fail below 50% total |
| 6 | `dco` | DCO (Signed-off-by on every PR commit) | every PR commit carries a `Signed-off-by:` trailer |
| 7 | `security-code` | govulncheck + CodeQL (Go) | reachable vuln symbol check + CodeQL go analysis |
| 8 | `security-deps` | Dependency review | GitHub dependency-review on PR dependency diffs (PR-event job) |
| 9 | `sbom-cve` | SBOM (Syft) + CVE scan (Grype, fail on critical) | SPDX + CycloneDX SBOM artifacts; fail on Critical CVEs |

Notes for the admin:
- `dco` and `security-deps` run only on `pull_request` events by design — that matches branch protection,
  which evaluates PRs, not direct pushes.
- `security-deps` requires the dependency graph + GitHub Advanced Security on this private repo
  (Settings -> Code security). Until an admin enables them, the job probes availability and passes with a
  warning annotation; it runs the strict dependency review automatically once the feature is on.
- The CodeQL half of `security-code` needs the same Advanced Security enablement (code scanning). Until then
  the job still enforces govulncheck on every PR and runs CodeQL automatically once the feature is on.
- Do NOT add `detect` (utility job for conditional gating), `performance`, `docker`, `helm`, or `functional`
  to the required list.

## Informational checks initially (report, do NOT require yet)

| Check name (job ID) | Activates when | Promote to required when |
|--------------------|----------------|--------------------------|
| `performance` | always runs; publishes results only when `Benchmark*` funcs exist | benchmark baselines land (ISI-6106) |
| `docker` | `detect` finds a `Dockerfile*` (**active now** — root `Dockerfile` present) | **now — ready to promote** (hardening landed, ISI-6212) |
| `helm` | `detect` finds a `Chart.yaml` (today: skips cleanly) | Helm chart lands (ISI-6104) |
| `functional` | `detect` finds files under `test/integration/` (today: skips cleanly) | envtest suite lands (ISI-6105) |

> **Admin action — promote `docker` to required (ISI-6212).** The `docker` job
> is now active and hardened: it builds `linux/amd64,linux/arm64` with
> provenance + SBOM attestation and runs the authoritative **built-image** Grype
> CVE gate (fail on Critical). On `main`/release it publishes a digest-pinned
> image to `ghcr.io/<org>/k8sagentmesh`; PRs are build-only (no push). Add
> `Docker build (multi-arch, scan, conditional GHCR push)` to the required
> status-check set. The `sbom-cve` required check retains the complementary
> source/dependency-tree CVE gate, so enforcement never regresses during the
> rollout.

Rationale: the conditional jobs self-skip today and activate via the `detect` job's outputs when their
Phase 2 artifacts land. Requiring them prematurely would block all PRs on jobs that cannot exercise
anything. When a Phase 2 artifact lands, its job auto-activates; at that point move it into the required
list (one-line change in the protection rule).

## Recommended additional settings (repo admin decision)

- Require a pull request before merging (the DCO and dependency-review jobs are PR-based).
- Include administrators.
- Do NOT enable "Require signed commits" — the project enforces DCO (`Signed-off-by` trailers), not GPG signing; the `dco` check covers it.
- Linear history: recommended (matches current single-line main history), optional.
- Require branches up to date before merging: enable after the first live PR cycle is green to avoid noisy rebases during rollout.

## Validation evidence (ISI-6102, completed 2026-10-18)

1. DONE — `actionlint` 1.7.7 clean over `.github/workflows/ci.yml` (only workflow file in the repo).
2. DONE — live test PR: https://github.com/K8sAgentMesh/K8sAgentMesh/pull/1, head `b86da60`, run 39106702257 —
   all 9 required jobs green (`build`, `unit`, `quality`, `guards`, `coverage`, `dco`, `security-code`,
   `security-deps`, `sbom-cve`); `docker`/`helm`/`functional` skip cleanly via `detect`; `performance`
   passes with 0 benchmarks. DCO probe: run 39104541132 on deliberately unsigned `82266c7` FAILED `dco`,
   green after the signoff fix (`ad39397`, run 39105399636).
3. DONE — `push` to main after merge (`7ceabbc`, run 39106786496) runs the same job set:
   all jobs green, `dco`/`security-deps` skipped (PR-only by design), conditional jobs skipped.

Failures triaged to concrete fixes along the way:
- `security-code`: job token needed `actions: read` for the CodeQL action's workflow-runs status call (commit 204c6d5);
  the CodeQL upload itself then requires GHAS (see the GHAS note above), so CodeQL is gated on the same
  availability probe (commit b86da60). `govulncheck` stays strict on every PR.
- `security-deps`: dependency review requires the dependency graph + GHAS on this private repo —
  self-activating probe added (commit 418b8bb); runs strictly once the features are enabled.

<!-- dco negative probe (ISI-6102): this commit is deliberately unsigned; expect the dco check to fail, then the signoff fix re-push to go green. -->
