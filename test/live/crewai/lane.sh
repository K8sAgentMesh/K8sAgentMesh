#!/usr/bin/env bash
# The live CrewAI lane (ISI-6547 phase 4) — real pinned stack end-to-end.
#
# Provisions an isolated uv venv with the pinned crewai[a2a] stack, then runs
# the Go lane test (test/live/crewai/lane_test.go, build tag crewai_lane),
# which drives the real crewaiRouter against a real Crew kickoff served by
# the pinned a2a-sdk JSON-RPC+SSE binding. The model backend is the lane's
# zero-credential OpenAI-compatible stand-in (mock_openai.py): the lane
# proves the mesh→crew wire, not a model.
#
# Usage:   test/live/crewai/lane.sh [logfile]
# Env:     CREWAI_LANE_PYTHON  reuse an existing venv python instead
#          HOLD_SECONDS        in-flight hold for the C1/cancel scenarios (default 6)
# Exit:    0 = lane green, nonzero = lane failed (log path printed).

set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../../.." && pwd)"
log="${1:-$here/lane.log}"
: "${HOLD_SECONDS:=6}"

# Refresh the pinned stack unless an existing interpreter was handed in.
if [[ -z "${CREWAI_LANE_PYTHON:-}" ]]; then
    venv="$here/.venv"
    if [[ ! -x "$venv/bin/python" ]] || ! "$venv/bin/python" -c 'import crewai, a2a, starlette, uvicorn' >/dev/null 2>&1; then
        echo "[lane] provisioning pinned stack in $venv (first run takes a few minutes)"
        uv venv --python 3.12 "$venv" >>"$log" 2>&1
        VIRTUAL_ENV="$venv" uv pip install "crewai[a2a]==1.15.28" >>"$log" 2>&1
    fi
    export CREWAI_LANE_PYTHON="$venv/bin/python"
fi
"$CREWAI_LANE_PYTHON" -c 'import crewai, a2a, starlette, uvicorn' >>"$log" 2>&1 || {
    echo "[lane] CREWAI_LANE_PYTHON lacks the pinned stack (crewai[a2a]==1.15.28, a2a-sdk~=0.3.10, starlette, uvicorn)" >&2
    exit 2
}

cd "$repo"
echo "[lane] stack: $($CREWAI_LANE_PYTHON -c 'import crewai; print(crewai.__version__)')"
echo "[lane] running Go lane test (logs: $log)"
if CREWAI_LANE=1 HOLD_SECONDS="$HOLD_SECONDS" go test -tags crewai_lane -count=1 -timeout 8m -v ./test/live/crewai/... 2>&1 | tee -a "$log"; then
    echo "[lane] GREEN — live CrewAI wire lane passed (log: $log)"
else
    echo "[lane] RED — see $log" >&2
    exit 1
fi
