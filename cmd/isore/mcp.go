package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/joaomdsg/isore/internal/proxy"
	"github.com/joaomdsg/isore/internal/serve"
	"github.com/joaomdsg/isore/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// openBrowser best-effort opens the user's default browser at rawURL —
// the fallback when no drivable Chromium exists.
func openBrowser(rawURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	_ = cmd.Start()
}

// proxyHolder owns the at-most-one in-process reverse proxy started via
// start_proxy. Every field access is under mu so a restart (which tears down
// the previous proxy) races safely with reload.
type proxyHolder struct {
	mu   sync.Mutex
	srv  *proxy.Server
	http *http.Server
	base string
}

// start binds listen synchronously — so "address already in use" surfaces to
// the caller rather than dying in a goroutine — then tears down any previous
// proxy and serves the new one in the background. Returns the base URL actually
// bound (honoring :0 in tests).
func (h *proxyHolder) start(target *url.URL, listen string, st *store.Store, overlay []byte, poll time.Duration, opts ...proxy.Option) (string, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return "", err
	}
	p := proxy.NewServer(target, st, overlay, poll, opts...)
	srv := &http.Server{Handler: p}
	base := "http://" + ln.Addr().String()

	h.mu.Lock()
	oldSrv, oldProxy := h.http, h.srv
	h.srv, h.http, h.base = p, srv, base
	h.mu.Unlock()

	if oldSrv != nil {
		_ = oldSrv.Close()
	}
	if oldProxy != nil {
		oldProxy.Close()
	}
	go srv.Serve(ln)
	return base, nil
}

// maxPortScan bounds how far startOnFreePort probes upward before giving up,
// so a machine with no nearby free port fails loudly instead of looping.
const maxPortScan = 64

// startOnFreePort binds the proxy at startPort, scanning upward past any port
// already held by another isore instance (EADDRINUSE) until one is free — so
// concurrent per-project instances on one machine don't collide. Returns the
// bound base URL and the port actually used. Non-EADDRINUSE bind errors surface
// immediately rather than being masked by the scan.
func (h *proxyHolder) startOnFreePort(target *url.URL, host string, startPort int, st *store.Store, overlay []byte, poll time.Duration, opts ...proxy.Option) (string, int, error) {
	var err error
	for port := startPort; port < startPort+maxPortScan; port++ {
		var base string
		base, err = h.start(target, fmt.Sprintf("%s:%d", host, port), st, overlay, poll, opts...)
		if err == nil {
			return base, port, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return "", 0, err
		}
	}
	return "", 0, fmt.Errorf("no free port in [%d,%d): %w", startPort, startPort+maxPortScan, err)
}

// reload refreshes every connected tab of the in-process proxy. Returns false
// when no proxy has been started, letting the caller fall back to an
// externally-run proxy's reload endpoint.
func (h *proxyHolder) reload() bool {
	h.mu.Lock()
	p := h.srv
	h.mu.Unlock()
	if p == nil {
		return false
	}
	p.Reload()
	return true
}

type listIn struct{}

type notesOut struct {
	Notes []serve.NoteView `json:"notes"`
}

type markFixedIn struct {
	ID      string `json:"id" jsonschema:"id of the note that has been fixed"`
	Summary string `json:"summary,omitempty" jsonschema:"one line for the user: what you changed and why"`
}

type markWorkingIn struct {
	ID string `json:"id" jsonschema:"id of the note you are starting on"`
}

type startProxyIn struct {
	TargetPort int `json:"targetPort,omitempty" jsonschema:"port your running dev server listens on and that isore will annotate; default 3000"`
	ProxyPort  int `json:"proxyPort,omitempty" jsonschema:"port to serve the annotated app on; default 4400"`
}

type startProxyOut struct {
	URL     string `json:"url"`
	Message string `json:"message"`
}

type emptyIn struct{}

type okOut struct {
	OK bool `json:"ok"`
}

type markFixedOut struct {
	Fixed string `json:"fixed"`
}

// instructions is surfaced to the connecting model on initialize. It describes
// the loop the agent runs itself: subscribe, fix each batch, repeat.
// No subagents — handing notes to workers loses the conversation's context.
const instructions = `Isore turns a user's live browser annotations into coding tasks. The user clicks an element in their running web app, writes a note about what to change, and hits Dispatch. You fix the code and their page updates live: badges turn blue the instant a note is dispatched, amber once you call mark_working, and green with your summary when done. The overlay freezes from the moment of dispatch (not just while you work), so the user cannot annotate or re-dispatch until you finish the batch — finish it promptly.

The user can attach follow-up comments to an already-dispatched note instead of rewriting it — read a note's "comments" field (oldest first) for that running context; the original "note" text never changes underneath you.

Do ALL of the fixing yourself, in the main conversation. Do NOT delegate dispatches — the actual code changes — to subagents or background workers: they lack your context about the codebase and the session.

1. SETUP (once): call start_proxy(targetPort, proxyPort). targetPort is the user's dev-server port; proxyPort is where the annotated app is served (defaults 3000 and 4400). It opens a driven Chromium window showing the overlay — the same window your browser_* tools drive. Its result includes the notes store path and the exact 'isore subscribe' command for step 2. If its result warns that no drivable browser was found, relay the warning and the install suggestion to the user before continuing. Then call list_notes and act (step 3) on anything already pending.

2. STAY CONNECTED WITH ONE PERSISTENT LISTENER: run the 'isore subscribe' command printed by start_proxy EXACTLY ONCE, as a single long-lived background listener (a Monitor), and keep it alive for the whole session. It opens ONE connection to the proxy's SSE stream and prints one JSON batch per dispatch the instant the user hits Dispatch, WITHOUT ever closing the channel — so each dispatch reaches you as a fresh line on that same open connection while you stay idle or work on something else. Do NOT re-run it per dispatch and do NOT run it in the foreground: one command, always connected, streaming every batch. Each printed line is one dispatch batch — parse it as a list of notes and act on it (step 3). The listener already checkpoints internally, so you never pass sinceMs or reprocess a batch. Relaunch it only if it exits — which happens when the proxy goes away; after any new start_proxy, start a fresh listener with the newly printed command (the proxy port may have changed). This CLI listener is the ONLY way to receive dispatches — there is no MCP tool for it.

3. ACT on the returned batch (all notes from one Dispatch). For each note: call mark_working(id) first (badge turns amber, so the user knows you picked it up); get_screenshot(id) to see exactly what the user saw at dispatch; use the note's note/selector/label/url to make the code change; verify against the live page with browser_screenshot / browser_eval / browser_html if useful; then mark_fixed(id, summary) with a short, user-facing summary (badge turns green, the summary shows in the overlay, and once every note is fixed the overlay thaws).

4. When the whole batch is fixed and the app has rebuilt, call reload_page once so the user's tabs refresh, then simply wait — your persistent listener from step 2 is still connected and will hand you the next dispatch as a new line. Never start a second listener.

5. THE CORRECTION LOOP: a fixed note is not necessarily final. The user can edit your summary in the overlay and hand it back — it comes back to you as a "correction" field on that note (your original summary is preserved separately). Treat a returned correction like a fresh note: read it as the user's own words describing what's still wrong or what to adjust, rework the fix accordingly, then call mark_fixed again with a new summary — this both resolves the correction and re-triggers the green badge/reload for that note. A note with no "correction" field needs no further action.

Notes: the store is safe under concurrent mark_working/mark_fixed calls. reload_page works only after start_proxy has been called. Keep summaries short — the user reads them in the overlay.`

func runMCP(args []string) error {
	fs := flag.NewFlagSet("isore mcp", flag.ExitOnError)
	out := fs.String("out", "isore-out/notes.json", "notes store shared with the harness")
	if err := fs.Parse(args); err != nil {
		return err
	}

	h := serve.New(store.New(*out), filepath.Dir(*out), func() int64 {
		return time.Now().UnixMilli()
	})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "isore",
		Title:   "Isore UI annotations",
		Version: "0.1.0",
	}, &mcp.ServerOptions{Instructions: instructions})

	holder := &proxyHolder{}
	ub := newUserBrowser()
	toolsPool := &driverPool{outDir: filepath.Dir(*out)}
	// Dedicated pool + mutex so dispatch-time element captures never interleave
	// navigations with the browser_* tools' pool (one driver, one tab).
	shootPool := &driverPool{outDir: filepath.Dir(*out)}
	var shootMu sync.Mutex
	shoot := func(pageURL, selector string) ([]byte, error) {
		shootMu.Lock()
		defer shootMu.Unlock()
		d, release, err := shootPool.get(context.Background())
		defer release()
		if err != nil {
			return nil, err
		}
		if err := d.Navigate(pageURL); err != nil {
			return nil, err
		}
		return d.Screenshot(selector)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notes",
		Description: "List pending UI annotations the user dispatched from the isore overlay: what to change, on which element (CSS selector). ALWAYS call get_screenshot for each note to see what the user saw, then fix and call mark_fixed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listIn) (*mcp.CallToolResult, notesOut, error) {
		ns, err := h.ListNotes(ctx)
		return nil, notesOut{Notes: ns}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_fixed",
		Description: "Mark a note as fixed once the requested change is implemented, with a one-line summary. The user sees the badge turn green and reads your summary in the overlay.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in markFixedIn) (*mcp.CallToolResult, markFixedOut, error) {
		if err := h.MarkFixed(ctx, in.ID, in.Summary); err != nil {
			return nil, markFixedOut{}, err
		}
		return nil, markFixedOut{Fixed: in.ID}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_working",
		Description: "Flag a note as picked up BEFORE you start changing code — the user's overlay badge turns amber so they know you are on it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in markWorkingIn) (*mcp.CallToolResult, okOut, error) {
		if err := h.MarkWorking(ctx, in.ID); err != nil {
			return nil, okOut{}, err
		}
		return nil, okOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_proxy",
		Description: "Start isore in proxy mode: run a reverse proxy that injects the annotation overlay in front of the user's dev server, and open a driven Chromium window to it (the same window the browser_* tools drive; falls back to the default browser with a warning if no Chromium/Chrome is installed). Pass the port your dev server listens on (targetPort) and the port to serve the annotated app on (proxyPort). Calling again restarts the proxy on the new ports. After it returns, run `isore subscribe` as a persistent background listener to receive the user's annotations.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in startProxyIn) (*mcp.CallToolResult, startProxyOut, error) {
		targetPort := in.TargetPort
		if targetPort == 0 {
			targetPort = 3000
		}
		proxyPort := in.ProxyPort
		if proxyPort == 0 {
			proxyPort = 4400
		}
		target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", targetPort))
		if err != nil {
			return nil, startProxyOut{}, err
		}
		requestedPort := proxyPort
		base, actualPort, err := holder.startOnFreePort(target, "127.0.0.1", proxyPort, store.New(*out), []byte(overlayJS), 300*time.Millisecond, proxy.WithShooter(shoot))
		if err != nil {
			return nil, startProxyOut{}, fmt.Errorf("start proxy near port %d: %w", requestedPort, err)
		}
		proxyPort = actualPort
		h.SetProxyBase(base)
		warning, d := ub.open(base)
		msg := fmt.Sprintf("Proxy live: annotating http://127.0.0.1:%d at %s. Notes store: %s — run `isore subscribe -out %s -proxyPort %d` ONCE as a single persistent background listener (a Monitor): it holds the SSE channel open and prints one JSON batch per dispatch without exiting, so you're notified of every dispatch on one connection. Do not re-run it per dispatch.", targetPort, base, *out, *out, proxyPort)
		if actualPort != requestedPort {
			msg += fmt.Sprintf(" (port %d was busy — another isore instance likely holds it — so I used %d instead.)", requestedPort, actualPort)
		}
		if d != nil {
			toolsPool.seed(d)
			msg += " A driven Chromium window is showing it — your browser_* tools drive that same window."
		} else {
			msg += " " + warning
		}
		msg += " Ctrl-Shift-N toggles the overlay; subscribe for dispatches."
		return nil, startProxyOut{URL: base, Message: msg}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reload_page",
		Description: "Refresh the user's browser tabs after you rebuilt the app. Call once your fix is live so the user sees it immediately. Requires start_proxy to have been called first.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, okOut, error) {
		if !holder.reload() {
			return nil, okOut{}, fmt.Errorf("no proxy running — call start_proxy first")
		}
		return nil, okOut{OK: true}, nil
	})

	addBrowserTools(server, h, toolsPool)

	fmt.Fprintf(os.Stderr, "isore mcp: serving stdio, store %s\n", *out)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
