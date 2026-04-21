#!/usr/bin/env bash
# Motion timeline integration test.
# Fires a fake UniFi motion webhook → checks SSE broadcast → verifies timeline entry.

set -euo pipefail

BASE="${WYZEFERAL_URL:-http://localhost:8082}"

echo "=== wyzeferal Motion Timeline test ==="
echo "    target: $BASE"
echo

PASS=0
FAIL=0

check() {
    local name="$1" result="$2" expect="$3"
    if [[ "$result" == *"$expect"* ]]; then
        echo "  PASS  $name"
        PASS=$((PASS+1))
    else
        echo "  FAIL  $name"
        echo "        expected: $expect"
        echo "        got:      ${result:0:300}"
        FAIL=$((FAIL+1))
    fi
}

# --- SSE content type ---
ct=$(curl -sf -I "$BASE/api/events" 2>/dev/null | grep -i "content-type" | head -1 || echo "")
check "GET /api/events content-type is event-stream" "$ct" "event-stream"

# --- Connect to SSE first, POST webhook, check output ---
SSE_TMP=$(mktemp)
# Read SSE stream for 5s in background
timeout 5 curl -sf -N "$BASE/api/events" > "$SSE_TMP" 2>/dev/null &
SSE_PID=$!
sleep 0.5  # let the connection establish

# POST fake motion webhook
PAYLOAD='{"events":[{"type":"smartDetectZone","smartDetectTypes":["person"],"camera":"Doorbell","start":1713600000000}]}'
r=$(curl -sf -X POST "$BASE/api/webhooks/unifi" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" 2>/dev/null || echo "WEBHOOK_FAILED")
check "POST /api/webhooks/unifi accepted" "$r" '"success"'

# Wait for SSE to receive the event
sleep 2
kill "$SSE_PID" 2>/dev/null || true
sse_out=$(cat "$SSE_TMP"); rm -f "$SSE_TMP"
check "SSE emits event after webhook" "$sse_out" "data:"

echo
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
[[ $FAIL -eq 0 ]]
