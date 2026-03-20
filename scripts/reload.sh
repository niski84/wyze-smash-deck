#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BINARY="$PROJECT_DIR/wyze-smash-deck"

echo "=== Wyze Smash Deck reload ==="
echo "→ Stopping existing process..."
pkill -f "$BINARY" 2>/dev/null && sleep 1 || echo "  (none running)"

if [ -f "$PROJECT_DIR/.env" ]; then
    set -a
    # shellcheck disable=SC1090
    source "$PROJECT_DIR/.env"
    set +a
fi

echo "→ Building..."
cd "$PROJECT_DIR"
go build -o "$BINARY" ./cmd/wyzeferal
echo "  Build OK: $BINARY"

PORT="${PORT:-8082}"
export PORT
echo "→ Starting on :${PORT}..."
nohup env PORT="$PORT" "$BINARY" >"$PROJECT_DIR/wyze-smash-deck.log" 2>&1 &
echo $! >"$PROJECT_DIR/wyze-smash-deck.pid"

for i in $(seq 1 30); do
  sleep 0.2
  if curl -fsS "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; then
    echo "✓ Wyze Smash Deck at http://127.0.0.1:${PORT}/"
    exit 0
  fi
done
echo "✗ Server did not respond — check wyze-smash-deck.log"
exit 1
