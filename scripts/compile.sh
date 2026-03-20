#!/bin/bash
# Build the wyze-smash-deck binary without restarting the server.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BINARY="$PROJECT_DIR/wyze-smash-deck"

cd "$PROJECT_DIR"
echo "→ Building wyze-smash-deck..."
go build -o "$BINARY" ./cmd/wyzeferal
echo "✓ Binary: $BINARY"
echo "  Run './scripts/reload.sh' to restart the server with the new build."
