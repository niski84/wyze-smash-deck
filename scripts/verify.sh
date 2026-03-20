#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -z "${PORT:-}" ]]; then
  PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()' 2>/dev/null)" || PORT="8082"
fi
export PORT
BASE="http://127.0.0.1:${PORT}"

echo "[verify] building wyzeferal..."
go build -o /tmp/wyzeferal-test ./cmd/wyzeferal

echo "[verify] starting server on ${PORT}..."
/tmp/wyzeferal-test > /tmp/wyzeferal-test.log 2>&1 &
PID=$!
trap 'kill ${PID} >/dev/null 2>&1 || true' EXIT

for _ in {1..30}; do
  if curl -fsS "${BASE}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 0.2
done

echo "[verify] GET /api/health"
curl -fsS "${BASE}/api/health" | grep -q '"success":true'

echo "[verify] GET /api/automations"
curl -fsS "${BASE}/api/automations" | grep -q '"success":true'

if [[ "${WYZE_SKIP_CLOUD:-}" != "1" ]]; then
  echo "[verify] GET /api/devices (needs DNS + openapi.wyze.com reachable)"
  curl -fsS "${BASE}/api/devices" | grep -q '"success":true'
fi

echo "[ok] wyzeferal API smoke test passed"
