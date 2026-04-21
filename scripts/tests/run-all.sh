#!/usr/bin/env bash
# Run all wyzeferal QA tests. Prints a summary and exits 0 only if all pass.
# Use SKIP_UI=1 to skip the camera UI test (requires browser-harness + API key).

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOTAL_FAIL=0

run_suite() {
    local script="$1"
    echo
    echo "──────────────────────────────────────────"
    if bash "$script"; then
        :
    else
        ((TOTAL_FAIL++))
    fi
}

run_suite "$DIR/test-api.sh"
run_suite "$DIR/test-motion-timeline.sh"

if [[ "${SKIP_UI:-0}" != "1" ]]; then
    run_suite "$DIR/test-camera-ui.sh"
else
    echo
    echo "  SKIP  camera UI tests (SKIP_UI=1)"
fi

echo
echo "══════════════════════════════════════════"
if [[ $TOTAL_FAIL -eq 0 ]]; then
    echo "  ALL SUITES PASSED"
else
    echo "  ${TOTAL_FAIL} SUITE(S) FAILED"
fi
echo "══════════════════════════════════════════"
exit "$TOTAL_FAIL"
