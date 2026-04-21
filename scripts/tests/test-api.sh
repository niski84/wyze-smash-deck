#!/usr/bin/env bash
# API smoke tests — no browser required.
# Exit 0 = all pass. Each check prints PASS/FAIL with detail.

set -euo pipefail

BASE="${WYZEFERAL_URL:-http://localhost:8082}"
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
        echo "        got:      $(echo "$result" | head -c 200)"
        FAIL=$((FAIL+1))
    fi
}

echo "=== wyzeferal API tests ==="
echo "    target: $BASE"
echo

# --- Health ---
r=$(curl -sf "$BASE/api/health" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET /api/health returns success:true"   "$r" '"success":true'

# --- Settings ---
r=$(curl -sf "$BASE/api/settings" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET /api/settings returns port field"   "$r" '"port"'
check "GET /api/settings has wyze_email"       "$r" '"wyze_email"'

# --- Camera info ---
r=$(curl -sf "$BASE/api/camera/info" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET /api/camera/info responds"          "$r" '"success"'

# --- Stream token: not stale ---
r=$(curl -sf "$BASE/api/camera/stream-token" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET /api/camera/stream-token success"          "$r" '"success":true'
check "GET /api/camera/stream-token has signaling_url" "$r" '"signaling_url"'
check "GET /api/camera/stream-token not stale"         "$r" '"stale":false'

# --- Force refresh ---
r=$(curl -sf "$BASE/api/camera/stream-token?force=true" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET /api/camera/stream-token?force=true success" "$r" '"success":true'

# --- Camera page HTML ---
r=$(curl -sf "$BASE/camera" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET /camera returns HTML"   "$r" '<html'
check "GET /camera has video tag"  "$r" '<video'

# --- Main page HTML ---
r=$(curl -sf "$BASE/" 2>/dev/null || echo "CONNECTION_FAILED")
check "GET / returns HTML"         "$r" 'Wyze'

echo
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
[[ $FAIL -eq 0 ]]
