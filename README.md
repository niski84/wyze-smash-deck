# Wyze Smash Deck

Self-hosted **Go + HTML** control panel for **Wyze** devices: quick toggles, dimmers, local nicknames, automations, and logs. Uses Wyze’s developer API (email/password + Key ID + API key) and talks to Wyze’s app endpoints (`auth-prod.api.wyze.com`, `api.wyzecam.com`).

> **Unofficial project.** Not affiliated with Wyze Labs. Use at your own risk.

## Features

- **Web UI** (`web/wyzeferal/`) — device grid with power toggles, **brightness sliders** for supported lights, **search**, **dark/light theme** (persisted in the browser).
- **Device list** — loads from Wyze, merged with optional **local registry** (custom display names, UI hints).
- **Power control** — `POST /api/devices/{mac}/control` with JSON `{ "model": "...", "on": true|false }`.
- **Brightness** — `POST /api/devices/{mac}/brightness` with `{ "model": "...", "brightness": 1-100 }` (mesh-style properties).
- **Rename (local)** — `PUT /api/devices/{mac}/name` updates the local display name in `data/wyzeferal-devices.json` (not Wyze cloud nickname).
- **Settings API** — `GET`/`POST /api/settings` for port and Wyze credentials; responses **mask** secrets; POST ignores masked placeholders so the UI can’t corrupt stored keys.
- **Automations** — CRUD under `/api/automations` with a scheduler (staggered steps, timers, safety-oriented flows — see `internal/wyzeferal/wyze_automation.go`).
- **Logs** — `GET /api/logs` for recent automation/server log lines.
- **Health** — `GET /api/health`.

## What gets stored locally (never commit)

| Path | Purpose |
|------|---------|
| `data/wyzeferal-settings.json` | Port, Wyze email/password, API keys, **access/refresh tokens** |
| `data/wyzeferal-devices.json` | Local metadata / display names |
| `data/wyzeferal-automations.json` | Automation definitions |
| `.env` | Optional env overrides for credentials and port |

These files are **gitignored**. Only the `*.example.json` templates belong in git.

## Requirements

- Go **1.22+** (see `go.mod`)
- Wyze **developer Key ID** and **API key** from [Wyze Developer API Console](https://developer.wyze.com/)
- Wyze account **email** and **password** (used only for token exchange; password is hashed per Wyze’s protocol before login)

## Quick start

```bash
git clone https://github.com/niski84/wyze-smash-deck.git
cd wyze-smash-deck

cp .env.example .env
# Edit .env: WYZE_EMAIL, WYZE_PASSWORD, WYZE_KEY_ID, WYZE_API_KEY

go run ./cmd/wyzeferal
# Open http://127.0.0.1:8082/
```

Or build and reload (rebuild + restart background server):

```bash
./scripts/reload.sh
```

Scripts:

| Script | Purpose |
|--------|---------|
| `scripts/compile.sh` | `go build` only |
| `scripts/reload.sh` | Build + restart server (default `:8082`) |
| `scripts/smoke.sh` | Curl health/settings/devices |
| `scripts/verify.sh` | Stricter checks (optional `WYZE_SKIP_CLOUD=1`) |
| `scripts/restore_data.sh` | Restore example JSON templates into `data/` |

## Configuration

- **Port:** `PORT` or `WYZE_FERAL_PORT`, or `port` in `wyzeferal-settings.json`.
- **Credentials:** Environment variables override JSON on load (`WYZE_EMAIL`, `WYZE_PASSWORD`, `WYZE_KEY_ID`, `WYZE_API_KEY`). After login, tokens may be persisted in settings JSON for fewer logins.

## Development

```bash
go test ./internal/wyzeferal/...
go build -o wyze-smash-deck ./cmd/wyzeferal
```

## Security notes

- Do **not** commit `.env` or anything under `data/` except examples.
- If you ever pasted real keys into a public issue or gist, **rotate** Wyze API keys and change your Wyze password.
- Run the server on a trusted network or behind authentication/reverse proxy if exposed beyond localhost.

## License

MIT — see [LICENSE](LICENSE).
