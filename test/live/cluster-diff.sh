#!/usr/bin/env bash
# cluster-diff.sh <before-dir> <after-dir> — the zero-control-plane
# assertion: every delta between the two snapshots must sit inside the
# allowlist (the mesh release's own footprint + the lane's ollama
# namespace). Anything else — any k8squad-shaped object in particular —
# fails the lane.
set -euo pipefail
before="$1"; after="$2"

# Allowlist regexes (anchored variants of the vanilla release's footprint).
ALLOW='^(mesh$|ollama$|meshpools\..*$|meshroutes\..*$|meshtelemetries\..*$|k8sagentmesh.*$|mesh-lv.*$)'

status=0
for f in namespaces.txt crds.txt clusterroles.txt clusterrolebindings.txt webhooks.txt; do
  [ -f "$before/$f" ] || continue
  delta="$(comm -13 "$before/$f" "$after/$f" || true)"
  removed="$(comm -23 "$before/$f" "$after/$f" || true)"
  if [ -n "$removed" ]; then
    echo "FAIL: control-plane objects REMOVED during the run ($f): $removed"
    status=1
  fi
  if [ -n "$delta" ]; then
    bad="$(printf '%s\n' "$delta" | grep -Ev "$ALLOW" || true)"
    if [ -n "$bad" ]; then
      echo "FAIL: unexpected control-plane ADDITIONS ($f):"
      printf '%s\n' "$bad"
      status=1
    fi
  fi
done

if grep -ri k8squad "$after" >/dev/null 2>&1; then
  echo "FAIL: k8squad objects present in the after-snapshot:"
  grep -ri k8squad "$after" || true
  status=1
fi

if [ "$status" -eq 0 ]; then
  echo "PASS: zero control-plane change outside the vanilla release footprint"
fi
exit $status
