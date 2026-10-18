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
- Do NOT add `detect` (utility job for conditional gating), `performance`, `docker`, `helm`, or `functional`
  to the required list.

## Informational checks initially (report, do NOT require yet)

| Check name (job ID) | Activates when | Promote to required when |
|--------------------|----------------|--------------------------|
| `performance` | always runs; publishes results only when `Benchmark*` funcs exist | benchmark baselines land (ISI-6106) |
| `docker` | `detect` finds a `Dockerfile*` (today: skips cleanly) | Docker build hardening lands (ISI-6103) |
| `helm` | `detect` finds a `Chart.yaml` (today: skips cleanly) | Helm chart lands (ISI-6104) |
| `functional` | `detect` finds files under `test/integration/` (today: skips cleanly) | envtest suite lands (ISI-6105) |

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

## Validation evidence (to be completed when repo access is granted)

1. DONE — `actionlint` 1.7.7 clean over `.github/workflows/ci.yml` (only workflow file in the repo).
2. PENDING — live test PR: all 9 required jobs green; `docker`/`helm`/`functional` skip cleanly; `performance` passes with 0 benchmarks; `dco` fails on a deliberately unsigned commit, green after signoff fix.
3. PENDING — `push` to main runs the same job set (`dco`/`security-deps` PR-only by design).

Blocked on: agent GitHub credential lacks access to K8sAgentMesh/K8sAgentMesh (403 `Write access not granted`);
confirmation card pending on ISI-6102 (interaction `47337c8d`), unblock owner: repo admin.
The ISI-6101 workflow commit (59f27a3) is also waiting on the same push access.

<!-- dco negative probe (ISI-6102): this commit is deliberately unsigned; expect the dco check to fail, then the signoff fix re-push to go green. -->
