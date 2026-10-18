#!/usr/bin/env bash
# settle-check.sh — assert the ISI-4224 settle-contract bar on a supervisor
# /task NDJSON transcript: the stream must END (this script only runs once
# the producing process exited — kubectl -i returned), and the last
# non-empty line must be a terminal event: {"kind":"completed"} or
# {"kind":"failed"} (the supervisor wire's TaskCompleted/TaskFailed).
# A transcript whose last line is a progress event, or that never closed
# (the producing step was timeout-killed upstream), is a FAIL, not a pass.
set -euo pipefail

transcript="$1"; lane="$2"

[ -f "$transcript" ] || { echo "FAIL($lane): transcript missing"; exit 1; }

# Last non-empty line.
last="$(grep -v '^[[:space:]]*$' "$transcript" | tail -n 1)"
if [ -z "$last" ]; then
  echo "FAIL($lane): empty transcript — no events at all"
  exit 1
fi

kind="$(printf '%s' "$last" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("kind",""))' 2>/dev/null || true)"
case "$kind" in
  completed|failed)
    echo "PASS($lane): terminal kind=$kind; stream closed after it"
    exit 0
    ;;
  *)
    echo "FAIL($lane): last line is not a terminal event (kind=${kind:-unparseable}): $last"
    exit 1
    ;;
esac
