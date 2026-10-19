#!/usr/bin/env bash
# gateway-probe.sh — timed /task HTTP probe against the gateway Service.
# Evidence-only (never fails the lane): the standing-process band behind
# this endpoint execs the unshipped `shim` binary (test/live/README.md
# gap 1), so the probe documents what actually happens vanilla — a
# connection-level answer, a 5xx, or a no-settle hang — for the record.
set -uo pipefail

SVC="$(kubectl -n mesh get svc -l app.kubernetes.io/instance=mesh -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [ -z "$SVC" ]; then
  echo "no gateway Service found (label app.kubernetes.io/instance=mesh) — nothing to probe"
  exit 0
fi
echo "probing http://127.0.0.1:18080/task via $SVC"
kubectl -n mesh port-forward "svc/$SVC" 18080:8080 >/dev/null 2>&1 &
PF=$!
sleep 3

BODY='{"id":"probe-1","message":{"role":"user","parts":[{"text":"say hello"}]}}'
set +e
curl -N -s --max-time 25 \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/x-ndjson' \
  -d "$BODY" \
  -w '\n[probe] http_code=%{http_code} exit_ok\n' \
  http://127.0.0.1:18080/task
rc=$?
set -e
kill $PF 2>/dev/null || true
echo "[probe] curl rc=$rc (28=timeout/no-settle, 7=connection refused — both are gap-1 evidence)"
exit 0
