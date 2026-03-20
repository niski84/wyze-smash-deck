#!/usr/bin/env bash
# Restore *template* Wyze data files if missing (cannot recover lost secrets from git).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

copy_if_missing() {
  local src="$1" dst="$2"
  if [[ ! -f "$dst" ]]; then
    cp "$src" "$dst"
    echo "→ created $dst from template"
  else
    echo "· skip $dst (already exists)"
  fi
}

echo "=== Wyze Feral Smash Deck — restore local data templates ==="
copy_if_missing "data/wyzeferal-settings.example.json" "data/wyzeferal-settings.json"
copy_if_missing "data/wyzeferal-devices.example.json" "data/wyzeferal-devices.json"
copy_if_missing "data/wyzeferal-automations.example.json" "data/wyzeferal-automations.json"

if ! grep -q '^WYZE_KEY_ID=' .env 2>/dev/null; then
  echo ""
  echo "Add to your .env (see .env.example):"
  echo "  WYZE_KEY_ID=..."
  echo "  WYZE_API_KEY=..."
fi

echo ""
echo "Paste Wyze keys from https://developer.wyze.com into .env and/or data/wyzeferal-settings.json"
echo "Then: ./scripts/reload_wyzeferal.sh"
echo "App: http://127.0.0.1:8082/"
