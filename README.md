# Wyze Smash Deck

A self-hosted web dashboard for controlling Wyze smart home devices directly through the Wyze cloud API.

Part of the [Smash Deck](https://github.com/niski84/smash-deck-catalog) family - self-hosted dashboards built in Go for the homelab.

## What It Does

Authenticates against Wyze's cloud endpoints (`auth-prod.api.wyze.com` and `api.wyzecam.com`) using developer credentials and gives you a single dashboard for every device on the account. Plugs, bulbs, and switches can be toggled instantly. Dimmable mesh bulbs get a brightness slider, and color-capable bulbs get a native color picker that writes the hex value straight to the device.

Devices can be tagged and filtered, viewed as a grid or a dense row list, and grouped into automations. The automation editor builds multi-step scenes with per-step delays, device selection, and actions like on, off, toggle, set color, and set brightness. Scenes can be triggered manually or scheduled to run on a recurring basis.

Settings, tags, and automations are stored in a local JSON file under `data/`. No Home Assistant or external broker required.

## Tech Stack

- Go (single binary, no runtime dependencies)
- Embedded vanilla HTML, CSS, and JavaScript (no framework)
- Docker / Compose support included

## Running

```bash
go build -o wyzeferal ./cmd/wyzeferal
./wyzeferal
```

Configure via environment variables (see `.env.example`). Requires a Wyze developer Key ID and API key from the Wyze developer console. Default port is 8082.

## Status

Active development.

## License

MIT
