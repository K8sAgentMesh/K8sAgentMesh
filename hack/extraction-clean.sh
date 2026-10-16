#!/usr/bin/env bash
# extraction-clean.sh — the P3 seam guard (ISI-5827): no package in this
# module may import github.com/K8squad/K8squad/*. The module is the generic
# AgentMesh surface; k8squad is host #1 behind the seam, never inside it.
#
# Exit 0 = clean; exit 1 = violation (prints offender + import line).
set -euo pipefail

cd "$(dirname "$0")/.."

# Forbidden import prefix. NOTE: this module's own module path
# (github.com/K8sAgentMesh/K8sAgentMesh) does not match the prefix, so the
# guard cannot false-positive on intra-module imports.
FORBIDDEN='github.com/K8squad/K8squad/'

violations="$(grep -rn --include='*.go' -F "\"${FORBIDDEN}" . || true)"
if [ -n "$violations" ]; then
  echo "FAIL: extraction-clean violations (k8squad imports in the module):"
  echo "$violations"
  exit 1
fi
echo "OK: module is extraction-clean (no k8squad imports)."
