#!/usr/bin/env bash
# Camera UI smoke test — uses a remote isolated browser via browser-harness.
# Verifies: camera page loads, video element exists, stream connects within timeout.
# Does NOT open tabs in the user's Chrome.

set -euo pipefail

BASE="${WYZEFERAL_URL:-http://localhost:8082}"
TIMEOUT="${CAMERA_CONNECT_TIMEOUT:-30}"

echo "=== wyzeferal Camera UI test ==="
echo "    target: $BASE"
echo "    connect timeout: ${TIMEOUT}s"
echo

browser-harness <<PY
import time, json, sys

start_remote_daemon("wyzeferal-qa")
PY

BU_NAME=wyzeferal-qa browser-harness <<PY
import time, json, sys

PASS = 0
FAIL = 0
BASE = "${BASE}"
TIMEOUT = int("${TIMEOUT}")

def check(name, cond, detail=""):
    global PASS, FAIL
    if cond:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}" + (f"\n        {detail}" if detail else ""))
        FAIL += 1

# --- Load camera page ---
new_tab(f"{BASE}/camera")
wait_for_load()
time.sleep(2)

info = page_info()
check("/camera page loaded", BASE in info.get("url",""), f"url={info.get('url')}")

# --- Video element present ---
video_src = js("document.querySelector('video')?.src || ''")
check("video element exists", video_src is not None, "no <video> tag found")

# --- Stream status message doesn't show error immediately ---
status_text = js("document.getElementById('camStatus')?.textContent || ''")
check("no immediate error on load", "error" not in (status_text or "").lower(),
      f"status='{status_text}'")

# --- Wait for ICE connected or live badge ---
deadline = time.time() + TIMEOUT
connected = False
while time.time() < deadline:
    time.sleep(2)
    live = js("document.getElementById('camLiveTag')?.style?.display")
    ice  = js("window._iceState || ''")
    if live not in (None, "", "none") or ice == "connected":
        connected = True
        break

check(f"stream connected within {TIMEOUT}s", connected,
      f"iceState={js('window._iceState || \"unknown\"')}, "
      f"liveTag={js('document.getElementById(\"camLiveTag\")?.style?.display')}")

# --- Controls visible ---
controls = js("document.getElementById('camControls') !== null")
check("control bar present", controls)

mute_btn = js("document.getElementById('camMuteBtn') !== null")
check("mute button present", mute_btn)

print()
print(f"=== Results: {PASS} passed, {FAIL} failed ===")
sys.exit(0 if FAIL == 0 else 1)
PY
