# Endpoints: wyze-smash-deck

**Base URL:** `http://localhost:8082`

All responses: `{ "success": bool, "data": any, "error": "..." }`

---

## Health

### `GET /api/health`
Returns service health.

**Response:**
```json
{ "success": true }
```

---

## Devices

### `GET /api/devices`
List all Wyze devices with current state (switch state, brightness, color, online status).

**Response:**
```json
{ "success": true, "data": [{ "mac": "80482C34FFE0", "display_name": "livingroom television power", "product_model": "WLPP1CFH", "is_online": true, "switch_state": 1 }] }
```

---

### `POST /api/devices/{mac}/control`
Turn a device on or off.

**Body:**
```json
{ "model": "WLPP1CFH", "on": true }
```

**Response:**
```json
{ "success": true, "data": { "ok": true } }
```

---

### `POST /api/devices/{mac}/brightness`
Set brightness on a dimmer plug (1–100).

**Body:**
```json
{ "model": "WLPP1CFH", "brightness": 80 }
```

**Response:**
```json
{ "success": true, "data": { "brightness": 80 } }
```

---

### `POST /api/devices/{mac}/color`
Set color on a color bulb (hex RGB).

**Body:**
```json
{ "model": "WLPA19C", "color": "#ff8800" }
```

**Response:**
```json
{ "success": true, "data": { "color": "#ff8800" } }
```

---

### `PUT /api/devices/{mac}/name`
Set a local display name for a device (stored in device registry, not sent to Wyze).

**Body:**
```json
{ "local_display_name": "Living Room TV" }
```

**Response:**
```json
{ "success": true, "data": { "saved": true } }
```

---

### `PUT /api/devices/{mac}/tags`
Update tags for a device.

**Body:**
```json
{ "tags": ["living room", "tv"] }
```

**Response:**
```json
{ "success": true, "data": { "saved": true, "tags": ["living room", "tv"] } }
```

---

## Automations

### `GET /api/automations`
List all automations (scenes, schedules, timers, safety rules).

**Response:**
```json
{ "success": true, "data": [{ "id": "abc123", "name": "Movie Night", "type": "scene", "enabled": true }] }
```

---

### `POST /api/automations/{id}/run`
Manually trigger an automation by ID.

**Response:**
```json
{ "success": true, "data": { "started": true } }
```

---

## Logs

### `GET /api/logs`
Fetch recent activity log lines. Query param: `?lines=200` (default 200, max 5000).

**Response:**
```json
{ "success": true, "data": { "lines": ["2026-04-01 20:00:00 turned on livingroom television power"] } }
```
