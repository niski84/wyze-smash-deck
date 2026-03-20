# Wyze Feral Smash Deck (package)

Go packages + HTTP UI for Wyze plugs/switches: **manual vs scheduled** automations, **scene steps with delay-before (stagger)**, multi-device actions with **stagger between devices**, timers, and safety flows.

## Run (from repo root)

```bash
go run ./cmd/wyzeferal
```

Default **:8082** (`PORT` / `WYZE_FERAL_PORT` / `data/wyzeferal-settings.json`).

## Deploy locally

```bash
./scripts/reload.sh
```

## Verify API

```bash
./scripts/verify.sh
```

See the repository [README.md](../../README.md) for features, security, and public setup.
