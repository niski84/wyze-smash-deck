#!/usr/bin/env bash
# Smoke-test Wyze Feral Smash Deck against a running server (default :8082).
# Health + settings must pass. /api/devices needs DNS + Wyze cloud (warn-only unless WYZE_STRICT_SMOKE=1).
set -euo pipefail
BASE="${WYZE_FERAL_BASE:-http://127.0.0.1:8082}"
STRICT="${WYZE_STRICT_SMOKE:-0}"

echo "[smoke] GET ${BASE}/api/health"
curl -fsS "${BASE}/api/health" | python3 -m json.tool | head -20

echo "[smoke] GET ${BASE}/api/settings (masked key ok)"
curl -fsS "${BASE}/api/settings" | python3 -m json.tool | head -20

echo "[smoke] GET ${BASE}/api/devices"
code="$(curl -sS -o /tmp/wyze-devices.json -w '%{http_code}' "${BASE}/api/devices")"
echo "HTTP ${code}"
python3 -m json.tool < /tmp/wyze-devices.json 2>/dev/null | head -40 || cat /tmp/wyze-devices.json

if [[ "${code}" != "200" ]]; then
  echo "[smoke] WARN: /api/devices HTTP ${code} (offline / DNS / firewall?)"
  [[ "${STRICT}" == "1" ]] && exit 1
  exit 0
fi

if ! grep -q '"success":true' /tmp/wyze-devices.json; then
  echo "[smoke] WARN: /api/devices returned success=false (see error above)"
  [[ "${STRICT}" == "1" ]] && exit 1
  exit 0
fi

python3 << 'PY'
import json
with open("/tmp/wyze-devices.json") as f:
    j = json.load(f)
data = j.get("data") or {}
print("[smoke] configured:", data.get("configured"), "devices:", len(data.get("devices") or []))
PY

echo "[smoke] OK"
