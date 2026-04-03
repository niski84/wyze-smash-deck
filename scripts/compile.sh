#!/bin/bash
# Build the wyze-smash-deck binary without restarting the server.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BINARY="$PROJECT_DIR/wyzeferal"

cd "$PROJECT_DIR"
echo "→ Building wyzeferal..."
go build -o "$BINARY" ./cmd/wyzeferal
echo "✓ Binary: $BINARY"
echo "  Run './scripts/reload.sh' to restart the server with the new build."
