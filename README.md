# isore

> **Research preview** — expect sharp edges and breaking changes.

Point at the ugly parts. Isore overlays your running web app: click any
element, leave a note, hit **Dispatch** — your coding agent picks the notes
up over MCP, fixes them, and your browser shows the progress live: badges
turn amber while the agent works, green with a summary when it's done, and
the page refreshes itself when the agent rebuilds.

## Install

```sh
go install github.com/joaomdsg/isore/cmd/isore@latest
```

## Use (proxy mode)

1. Hook your agent up once, from your project directory:

   ```sh
   claude mcp add isore -- isore mcp
   ```

2. Ask your agent to start isore in front of your dev server. It calls the
   `start_proxy` tool, which stands up a reverse proxy that injects the overlay
   and opens a driven Chromium window to it — the very window the agent's
   browser tools see and drive. (No Chromium/Chrome installed? It falls back
   to your default browser and tells the agent to suggest installing one.)

   > *"start isore in front of my app on :3000, then await my notes and fix them"*

   ```
   start_proxy(targetPort=3000, proxyPort=4400)   # your browser opens at http://127.0.0.1:4400
   ```

3. Toggle the overlay (bottom-right), click elements, write notes,
   **Dispatch**. The agent picks them up, fixes them, and refreshes your page.

The agent doesn't just read your notes — it gets the browser:

- **Control**: `start_proxy` (launch the proxy + open your browser — start
  here; call again to move ports), `reload_page` (refresh your tabs after a
  rebuild — needs `start_proxy` first).
- **Notes**: `await_notes` (blocks until you dispatch — the trigger),
  `list_notes`, `mark_working` (badge turns amber), `mark_fixed` + summary
  (badge turns green, summary shows in the overlay).
- **Eyes**: `get_screenshot` returns the element PNG captured at dispatch;
  `browser_screenshot` captures the live page or element to verify a fix.
- **Hands**: `browser_eval` (run JS — read computed styles/state, click,
  type, drive the UI),
  `browser_html` (live DOM), `browser_navigate`, `browser_console`
  (logs/warnings/errors).

`start_proxy` runs the proxy inside the `isore mcp` server — no second
process to manage. For the element screenshots captured on every dispatch it
attaches to your harness browser if one is running (see below), otherwise it
launches its own headless Chromium. Browser ↔ proxy is plain HTTP + SSE;
websockets and your dev server's HMR stream pass through untouched.

## Harness mode

`isore -url http://localhost:3000` opens a managed, visible Chromium
with the overlay injected. It exposes its CDP endpoint
(`-debug-port`, default 9222) via `isore-out/browser.json`, so the
agent's browser tools attach to the very browser you're looking at.
Flags: `-out` store path, `-chrome` browser binary.

Notes live in `isore-out/notes.json`; every command takes `-out`.

MIT.
