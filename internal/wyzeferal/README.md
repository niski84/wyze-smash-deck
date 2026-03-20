# Wyze Feral Smash Deck

Go packages + HTTP UI for Wyze plugs/switches: **manual vs scheduled** automations, **scene steps with delay-before (stagger)**, simple multi-device actions with **stagger between devices**, timers, and safety flows.

## Run (from repo root `rv sale`)

```bash
go run ./cmd/wyzeferal
```

Default **:8082** (`PORT` / `WYZE_FERAL_PORT` / `data/wyzeferal-settings.json`).

## Deploy locally

```bash
./scripts/reload_wyzeferal.sh
```

## Verify API

```bash
./scripts/verify_wyzeferal.sh
```

## Plex split

The **Plex Library Dashboard** lives in `~/goprojects/plex-dashboard`; this app is separate.
