# Performance benchmarks & CI comparison gate

The CI `performance` job (job 9) is a **comparison gate** (ISI-6106 Phase 2). It
runs the Go micro-benchmarks on every PR and fails the check if a benchmark
regresses past the alert threshold relative to the stored baseline.

## Benchmarks

Defined in `mesh/routeselect_bench_test.go` (ISI-6252), on the real per-dispatch
route-selection hot path — no synthetic gate-gaming:

| Benchmark              | Covers                                                        |
| ---------------------- | ------------------------------------------------------------- |
| `BenchmarkRoute`       | per-dispatch route decision: flag check, registry lookup, URL |
| `BenchmarkIsSaturation`| 409-refusal classification on the re-drive path               |
| `BenchmarkNewTeamSet`  | canary-list parsing at config load / reload                   |

## How the gate works

- **Command:** `go test ./... -run='^$' -bench=. -benchmem -cpu=4 -count=1`.
  `-cpu=4` pins the `-4` name suffix so results match across runners
  (benchmark-action keys on the full name including suffix).
- **Baselines** are stored by [benchmark-action](https://github.com/benchmark-action/github-action-benchmark)
  on the **`gh-pages`** branch under `dev/bench/`. They are generated on
  CI-runner hardware by the `main`-branch run (`auto-push: true`) so PR numbers
  compare like-for-like. A locally captured baseline is **not** valid across
  machines and must never be committed as the gate baseline.
- **Pull requests** compare against that baseline and **fail** if any benchmark
  is ≥ `200%` of baseline (2× slower). The threshold is deliberately loose to
  absorb shared-runner noise while still catching real regressions. A comment is
  posted on the PR on regression. PR runs never push.
- **Fail-open bootstrap:** before the first `main` run populates `dev/bench/`,
  there is no prior data point, so the gate passes (no alert). The gate becomes
  active automatically after the first merge to `main`.

## Reference baseline (local, informational only)

Captured locally at `main@bfa4607`+ with `-cpu=4 -count=6` (NOT the CI baseline;
absolute ns/op depends on hardware — these are for sanity only):

| Benchmark               | ~ns/op | B/op | allocs/op |
| ----------------------- | -----: | ---: | --------: |
| `BenchmarkRoute-4`      |   ~41  |   32 |         1 |
| `BenchmarkIsSaturation-4`| ~29   |    0 |         0 |
| `BenchmarkNewTeamSet-4` |  ~215  |  336 |         3 |

## Refreshing baselines after a legitimate perf change

Baselines auto-refresh on every merge to `main` (the `main` run pushes the new
numbers to `gh-pages`). No manual step is required for normal changes. If a PR
intentionally changes performance, expect the gate to flag it; land it via the
normal review path and the next `main` run re-baselines automatically.

## Making the gate required

The job fails the check on regression, which surfaces as a red check on the PR.
To make it **blocking**, add `Benchmarks (comparison gate)` to the repo's
branch-protection required status checks for `main` (GitHub admin setting; not
expressible in the workflow file).
