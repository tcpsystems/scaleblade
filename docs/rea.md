# REA browser capture: setup and result

REA was used to try to capture the authenticated portal page at runtime. The
browser session worked; REA's own scenario provider failed. This page records
the exact setup and outcome so the gap is auditable and reproducible.

## Goal

Capture `https://my.scaleblade.com/compute/bare-metal` with:

- the real browser session cookies,
- network request metadata (URLs, methods, status),
- console/page errors,
- bounded response bodies disabled (to avoid storing account data),
- no raw capture committed to the repository.

## Tooling

- REA 6.7.0 via `npx rea-agents@6.7.0 mcp`, CDP browser provider.
- Chrome for Testing:
  `/home/kivox/.agent-browser/browsers/chrome-155.0.8059.84/chrome`
- A scratch cookie JSON file at the documented default location, mode `0600`.
  Cookie values were read by a local script; they were never printed.

## Setup

### 1. Start an isolated headless Chrome

```bash
CHROME=/home/kivox/.agent-browser/browsers/chrome-155.0.8059.84/chrome
"$CHROME" \
  --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --remote-debugging-address=127.0.0.1 --remote-debugging-port=9229 \
  --user-data-dir=/tmp/sb-rea-profile about:blank
```

The profile is temporary and outside the repository.

### 2. Inject the session cookies over CDP

A small Node script read the `0600` JSON file and sent
`Network.setCookies` to the page target's WebSocket. Only the count was printed:

```
cookies_injected 2 cookies_visible_for_origin 2
```

No cookie name or value was written to disk or output. This is the same
technique as loading a browser profile, but the secrets stay in memory and in
Chrome's own profile store.

### 3. Confirm the authenticated page

```bash
curl -sS http://127.0.0.1:9229/json/list
```

```json
[{"type":"page","url":"https://my.scaleblade.com/compute/bare-metal","title":"Scaleblade Portal"}]
```

A CDP `Page.navigate` to `https://my.scaleblade.com/` loaded the portal, and the
app redirected to `/compute/bare-metal`. This is a runtime observation: the
cookie pair was accepted by a fresh Chrome profile, and the page rendered under
the authenticated session.

### 4. Ask REA to capture it

MCP call (sanitized; endpoint and target are loopback-only):

```json
{
  "browser": {
    "mode": "connect",
    "cdp_endpoint": "http://127.0.0.1:9229",
    "target_id": "<page-target>"
  },
  "start_url": {"url": "https://my.scaleblade.com/compute/bare-metal"},
  "actions": [{"step_id": "settle", "action": "wait_for_timeout", "duration_ms": 6000}],
  "capture": {
    "at_end": ["url"],
    "events": ["console", "page-errors", "network"],
    "network": {"request_body": false, "response_body": false, "header_values": false}
  },
  "environment": {
    "viewport": {"width": 1280, "height": 720, "device_scale_factor": 1},
    "locale": "en-US",
    "timezone": "UTC",
    "color_scheme": "light",
    "reduced_motion": "reduce",
    "service_workers": "block"
  }
}
```

The same scenario via the REA CLI:

```bash
node scripts/rea.mjs capture-browser-scenario /tmp/sb-scenario.json --format json
```

## Result

Both calls failed identically:

```json
{
  "error": "Browser scenario capture failed",
  "code": "execution_failure",
  "category": "execution_failure",
  "message": "Analysis could not complete. Run `rea doctor` and review the reported details; repeating the unchanged request is not expected to succeed.",
  "retryable": false,
  "details": {
    "provider_id": "rea-playwright-browser-scenario",
    "operation": "capture_browser_scenario"
  }
}
```

`rea doctor --json` reported Node 22.23.1 and the Fedora host as healthy. It
also reports missing native analysis engines (Hopper, Ghidra, IDA, JADX, Kuna),
which are unrelated to browser capture. No detailed cause for
`rea-playwright-browser-scenario` was returned, and the failure is marked
non-retryable, so the capture was not repeated indefinitely.

Checks that passed before asking REA:

- Chrome 155 CDP endpoint was reachable on loopback.
- The page target was listed by REA's `list_browser_targets` with origin
  `https://my.scaleblade.com`.
- The page had already loaded under the authenticated session.

Checks that could not be completed:

- Playwright `connectOverCDP` attach and scenario actions.
- Network/console capture inside REA.
- A sanitized capture artifact.

## What was stored

Nothing from the capture attempt is in the repository. There is no HAR, PCAP,
screenshot, DOM dump, console log, cookie jar, or raw REA bundle in the tree.
The authenticated page-load fact and the error above are the only runtime
evidence recorded, and they are written here by hand.

The scratch browser profile (`/tmp/sb-rea-profile`), scenario JSON, and helper
scripts were local and disposable. They contained no cookie values; the cookie
file itself was never copied. Raw response bodies were disabled in the capture
request, so even a successful capture would not have contained API payloads.

## Reproducing without secrets in the repository

1. Build the binaries and create the `0600` cookie JSON as in the README.
2. Repeat steps 1–3 with your own browser/profile and your own cookie file.
3. Keep the profile, capture, and any scenario output outside this repository.
4. If REA's provider works in your environment, inspect the result, strip any
   account data, and add only a short sanitized summary here. Do not commit the
   raw capture.

## Fallback used for the catalogue

Because the runtime capture was unavailable:

- Endpoint paths and methods come from the Next.js chunks.
- Existence, status codes and response field names come from authenticated
  read-only `GET` requests.
- Everything not actually executed is labelled `inferred` in
  [api.md](api.md); admin routes are additionally marked `gated`.

See [notes.md](notes.md) for the full methodology and its limitations.
