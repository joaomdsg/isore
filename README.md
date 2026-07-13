# eyesore

> **Research preview** — expect sharp edges and breaking changes.

Point at the ugly parts. Eyesore overlays your running web app: click any
element, leave a note, hit **Dispatch** — your coding agent picks the notes
up over MCP, fixes them, and your browser shows the progress live: badges
turn amber while the agent works, green with a summary when it's done, and
the page refreshes itself when the agent rebuilds.

## Install

```sh
go install github.com/joaomdsg/eyesore/cmd/eyesore@latest
```

## Use (proxy mode)

1. Put eyesore in front of your dev server and browse through it:

   ```sh
   eyesore proxy -url http://localhost:3000
   # browse http://127.0.0.1:4400 in any browser
   ```

2. Hook your agent up (once, from the same directory):

   ```sh
   claude mcp add eyesore -- eyesore mcp
   ```

3. Toggle the overlay (bottom-right), click elements, write notes,
   **Dispatch**. Tell your agent: *"await eyesore notes and fix them"*.

The agent doesn't just read your notes — it gets the browser:

- **Notes**: `await_notes` (blocks until you dispatch — the trigger),
  `list_notes`, `mark_working` (badge turns amber), `mark_fixed` + summary
  (badge turns green, summary shows in the overlay), `reload_page`.
- **Eyes**: `get_screenshot` returns the element PNG captured at dispatch;
  `browser_screenshot` captures the live page or element to verify a fix.
- **Hands**: `browser_eval` (run JS, read computed styles/state),
  `browser_html` (live DOM), `browser_navigate`, `browser_console`
  (logs/warnings/errors).

In proxy mode the MCP drives its own headless Chromium (also used to
capture element screenshots on every dispatch). Browser ↔ proxy is plain
HTTP + SSE; websockets and your dev server's HMR stream pass through
untouched.

## Harness mode

`eyesore -url http://localhost:3000` opens a managed, visible Chromium
with the overlay injected. It exposes its CDP endpoint
(`-debug-port`, default 9222) via `eyesore-out/browser.json`, so the
agent's browser tools attach to the very browser you're looking at.
Flags: `-out` store path, `-chrome` browser binary.

Notes live in `eyesore-out/notes.json`; every command takes `-out`.

## Roadmap

Deeper context capture (component source via sourcemaps) · agent
self-verification with before/after screenshots · flow recording →
reproducible tests.

MIT.
