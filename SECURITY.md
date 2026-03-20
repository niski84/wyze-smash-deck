# Security

## Reporting

If you find a security issue in this **unofficial** Wyze helper app, please open a **private** security advisory on GitHub or contact the repository owner directly. Do not post credentials, tokens, or live API responses in public issues.

## Operational hygiene

- **Never commit** `.env`, `data/wyzeferal-settings.json`, `data/wyzeferal-devices.json`, `data/wyzeferal-automations.json`, or any file containing tokens.
- **Rotate** Wyze developer API keys and your Wyze account password if they may have leaked.
- Prefer running on **localhost** or behind a reverse proxy with authentication if the service is reachable from untrusted networks.
