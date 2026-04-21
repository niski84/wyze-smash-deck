# Wyze WebRTC Stream Auth — Reverse Engineering Guide

A developer's field notes on implementing pure-Go, browser-free WebRTC credential refresh
for the Wyze doorbell camera. Written for anyone who wants to do this without AI tooling,
Playwright, or a browser running in the background.

---

## The Problem

You want to stream your Wyze doorbell camera in a web app. Wyze's mobile API gives you an
`access_token` that works for most API calls — but it does **not** work for WebRTC. The
`get-streams` endpoint rejects it with `INVALID_PARAMETER`. Wyze uses a separate OAuth2
JWT for their web app, and the two are not interchangeable.

The naïve path is to run a headless browser, log into `my.wyze.com`, and intercept the
network request. That works, but it's fragile, heavy, and breaks when Wyze updates their
JS bundle. The right path is to reverse-engineer the auth pipeline and call the APIs
directly from Go.

---

## Phase 1 — Understanding the Stack

**What you're up against:**

Wyze's web app (`my.wyze.com`) is a Next.js app backed by a Flask-based BFF (backend for
frontend) running at `services.wyze.com`. The BFF holds all the OAuth2 secrets. The
browser never sees the `client_secret` — it only receives cookies.

The two credential systems are:

| System | Where it's used |
|---|---|
| Mobile API token (`access_token` from `auth.wyze.com`) | Device list, device control, snapshots |
| Web OAuth2 JWT (from `services.wyze.com`) | WebRTC `get-streams`, web-only APIs |

**Key insight:** Don't try to use the mobile token for `get-streams`. It won't work. You
need the web JWT. These are different tokens with different issuers.

---

## Phase 2 — Finding the JWT Refresh Endpoint

The web app calls an internal BFF to refresh its JWT. The BFF does the OAuth2 dance with
`client_secret` on behalf of the browser. From the browser's perspective, it just sends a
session cookie and gets a fresh JWT back.

**The endpoint:**

```
GET https://services.wyze.com/api/v2/santa/refresh?tag=webview
Cookie: session=<flask_session_value>
```

**Response:**
```json
{"code": 1, "access_token": "eyJ...", "email": "you@example.com"}
```

The `session` cookie is a Flask (itsdangerous) signed session. The actual value is a
base64-encoded, zlib-compressed JSON blob containing `access_token`, `refresh_token`,
`userId`, and more. You can decode it (for inspection) like this in Python:

```python
import base64, zlib, json
cookie = "your_session_value_here"
padded = cookie + "=" * (-len(cookie) % 4)
raw = base64.urlsafe_b64decode(padded)
decoded = zlib.decompress(raw[1:])  # first byte is a zlib flag
print(json.loads(decoded))
```

**How to get the session cookie:**

1. Log into `my.wyze.com` in Chrome
2. Open DevTools → Application → Cookies → `services.wyze.com`
3. Copy the `session` cookie value

This cookie is permanent (`_permanent: true`, ~7-day `_remember_seconds`). You only need
to re-extract it when it expires or when you log out.

**Why you can't call this from a browser:**

`services.wyze.com` returns CORS headers that block `fetch()` calls from the browser.
Server-side only. Go (or curl) works fine.

---

## Phase 3 — The `get-streams` Endpoint

Once you have a web JWT, you can call `get-streams` directly:

```
POST https://app.wyzecam.com/app/v4/camera/get-streams
Content-Type: application/json
Authorization: <jwt>
access_token: <jwt>
appid: strv_e7f78e9e7738dc50
appinfo: wyze_web_2.3.1
signature2: <computed — see below>
requestid: <any small random int>
```

**Request body:**
```json
{
  "device_list": [{
    "device_id": "GW_DBD_80482C640246",
    "device_model": "GW_DBD",
    "provider": "webrtc",
    "parameters": {"use_trickle": true}
  }],
  "nonce": 1713600000000
}
```

`nonce` is `time.Now().UnixMilli()`. `device_id` is your doorbell's MAC-based ID (find it
via `ListDevices` from the mobile API). `device_model` for the Video Doorbell Pro is
`GW_DBD`.

**Response (success, `code: "1"`):**
```json
{
  "code": "1",
  "data": [{
    "device_id": "GW_DBD_80482C640246",
    "params": {
      "signaling_url": "wss://wyze-mars-webcsrv.wyzecam.com?token=...",
      "ice_servers": [
        {"url": "turn:...", "username": "...", "credential": "..."},
        {"url": "stun:..."}
      ]
    }
  }]
}
```

---

## Phase 4 — The `signature2` Header (The Hard Part)

The `signature2` header is a request integrity check. Wyze's web app JS computes it at
runtime. This is the field most likely to block you if you don't know the formula.

**How it was found:** The formula was reverse-engineered from the Wyze web app's JS bundle
(Webpack module 59807, function `pg`). You can find it by searching the bundle for
`signature2` or `HMAC`.

**Formula:**
```
intermediate = MD5( accessToken + "gbJojEBViLklgwyyDikx5ztSvKBXI5oU" )
signature2   = HMAC-MD5( json_body_string, hex(intermediate) )
```

Both steps use MD5. Yes, MD5. Wyze's choice, not ours — this is a protocol requirement,
not a security decision on your part.

**Go implementation:**
```go
func computeSignature2(accessToken, jsonBody string) string {
    // #nosec G401 - MD5 required by Wyze protocol
    mid := md5.Sum([]byte(accessToken + "gbJojEBViLklgwyyDikx5ztSvKBXI5oU"))
    midHex := hex.EncodeToString(mid[:])
    mac := hmac.New(md5.New, []byte(midHex)) // #nosec G401
    mac.Write([]byte(jsonBody))
    return hex.EncodeToString(mac.Sum(nil))
}
```

**Critical detail:** The `jsonBody` string must be exactly the JSON you send in the
request body — same byte-for-byte. Use `json.Marshal` once, convert to string, pass to
both the body reader and this function.

**Why `code != "1"` despite a valid signature:**  
If `get-streams` returns a non-"1" code even though your signature looks right, your JWT
is likely stale. The JWT has a 2-hour TTL but can be invalidated earlier. Force a refresh
and retry.

---

## Phase 5 — TTLs and the Refresh Schedule

There are two TTLs to manage:

| Token | TTL | Refresh strategy |
|---|---|---|
| Web JWT (`access_token`) | ~2 hours | Call `services.wyze.com/api/v2/santa/refresh` every 110 min |
| TURN credentials (inside `get-streams` response) | ~5 minutes | Call `get-streams` every 3 minutes |

The TURN credential TTL is the tight one. If you let `ice_servers` go stale, WebRTC
connection establishment will fail silently. Refresh well within the 5-minute window.

**Refresh schedule that works:**
- JWT: refresh after 110 minutes (10-minute safety margin on the 2-hour TTL)
- Stream params: refresh every 3 minutes; serve stale up to 4 minutes if refresh fails

---

## Phase 6 — Go Architecture

The implementation splits into two types:

**`WyzeStreamClient`** — handles auth and API calls:
- Holds the session cookie and current JWT
- `ensureJWT(ctx)` — refreshes JWT when it approaches expiry, persists new JWT to disk
- `GetStreamParams(ctx, deviceID, deviceModel)` — calls `get-streams`, returns `*StreamParams`

**`streamRefresher`** — background goroutine + in-memory cache:
- Wraps `WyzeStreamClient`
- Starts a ticker loop; calls `refresh()` on every tick
- `get()` returns the latest `*StreamParams` under a read lock
- HTTP handler reads from this in-memory cache (no network call on the request path)

```
HTTP request → streamRefresher.get() → cached *StreamParams (μs)
Background goroutine → GetStreamParams() → Wyze API (every 3 min)
```

This pattern means your frontend never waits for a Wyze API call. The background goroutine
absorbs all latency and failure cases.

**Wiring it up in the HTTP server:**
```go
// On startup (if session cookie is configured):
streamClient := NewWyzeStreamClient(sessionCookie, cachedJWT, func(jwt string) {
    // persist refreshed JWT to settings file
})
s.streamRefresher = newStreamRefresher(streamClient, deviceID, deviceModel)

// In main() after server is constructed:
srv.StartStreamRefresher(ctx)  // launches background goroutine
```

---

## Pitfalls and Things That Burned Us

**1. Mobile API token ≠ web JWT**  
The mobile `access_token` from `POST auth.wyze.com/oauth2/token` will always return
`INVALID_PARAMETER` from `get-streams`. Don't waste time trying to make it work.

**2. OAuth2 ROPC doesn't work**  
`POST auth.wyze.com/oauth2/token` with `grant_type=password` returns "unauthorized" because
the web client's `client_secret` is stored server-side in `services.wyze.com` and never
exposed to the browser. You cannot replicate the full OAuth2 flow without that secret.
The session cookie BFF approach is the correct path.

**3. CORS blocks browser-side refresh**  
`fetch('https://services.wyze.com/...')` from `my.wyze.com` fails with CORS. The call must
be made server-side. Go and curl work; browser JS does not.

**4. `signature2` body must be identical to the sent body**  
Compute the signature from `json.Marshal(reqBody)` before creating the `http.Request`. If
you re-marshal or reorder keys at any point, the signature will be wrong and you'll get
cryptic auth errors.

**5. JSON body must be a string for HMAC, not bytes**  
The HMAC input is `string(bodyBytes)` — convert explicitly. `json.Marshal` returns `[]byte`;
pass it through `string()` for the signature, then use `bytes.NewReader(bodyBytes)` for
the HTTP body.

**6. `code` field in `get-streams` response is a string, not an int**  
Success is `"1"` (string). If you compare to integer `1` you'll silently treat all responses
as errors.

**7. Session cookie expiry**  
The Flask session is set with `_remember_seconds: 604800` (~7 days). When it expires, you
need to log back into `my.wyze.com` and re-extract the cookie. There's no programmatic
way to get a new one — this is a manual bootstrapping step that happens roughly weekly.
Consider adding a health check endpoint that reports the age of the session cookie.

---

## Quick Reference

| What | Value |
|---|---|
| JWT refresh endpoint | `GET https://services.wyze.com/api/v2/santa/refresh?tag=webview` |
| Stream params endpoint | `POST https://app.wyzecam.com/app/v4/camera/get-streams` |
| `signature2` secret | `gbJojEBViLklgwyyDikx5ztSvKBXI5oU` |
| App ID header | `strv_e7f78e9e7738dc50` |
| App info header | `wyze_web_2.3.1` |
| Doorbell device model | `GW_DBD` |
| JWT TTL | ~2 hours |
| TURN credential TTL | ~5 minutes |
| Session cookie TTL | ~7 days |
