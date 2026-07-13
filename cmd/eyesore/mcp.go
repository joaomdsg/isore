package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joaomdsg/eyesore/internal/serve"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listIn struct{}

type notesOut struct {
	Notes []serve.NoteView `json:"notes"`
}

type awaitIn struct {
	SinceMs        int64 `json:"sinceMs,omitempty" jsonschema:"only notes dispatched after this unix-ms timestamp; 0 means anything dispatched from now on"`
	TimeoutSeconds int   `json:"timeoutSeconds,omitempty" jsonschema:"how long to wait before returning empty; default 120"`
}

type markFixedIn struct {
	ID      string `json:"id" jsonschema:"id of the note that has been fixed"`
	Summary string `json:"summary,omitempty" jsonschema:"one line for the user: what you changed and why"`
}

type markWorkingIn struct {
	ID string `json:"id" jsonschema:"id of the note you are starting on"`
}

type emptyIn struct{}

type okOut struct {
	OK bool `json:"ok"`
}

type markFixedOut struct {
	Fixed string `json:"fixed"`
}

func runMCP(args []string) error {
	fs := flag.NewFlagSet("eyesore mcp", flag.ExitOnError)
	out := fs.String("out", "eyesore-out/notes.json", "notes store shared with the harness")
	proxyURL := fs.String("proxy", "http://127.0.0.1:4400", "eyesore proxy base URL (for reload_page and live status)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	h := serve.New(store.New(*out), filepath.Dir(*out), func() int64 {
		return time.Now().UnixMilli()
	})
	if *proxyURL != "" {
		h.ReloadURL = *proxyURL + "/__eyesore/reload"
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "eyesore",
		Title:   "Eyesore UI annotations",
		Version: "0.1.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notes",
		Description: "List pending UI annotations the user dispatched from the eyesore overlay: what to change, on which element (CSS selector). ALWAYS call get_screenshot for each note to see what the user saw, then fix and call mark_fixed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listIn) (*mcp.CallToolResult, notesOut, error) {
		ns, err := h.ListNotes(ctx)
		return nil, notesOut{Notes: ns}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "await_notes",
		Description: "Block until the user dispatches new annotations from the eyesore overlay, then return them. Empty result means the wait timed out — call again to keep listening.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in awaitIn) (*mcp.CallToolResult, notesOut, error) {
		timeout := 120 * time.Second
		if in.TimeoutSeconds > 0 {
			timeout = time.Duration(in.TimeoutSeconds) * time.Second
		}
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ns, err := h.Await(waitCtx, in.SinceMs)
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
		Name:        "reload_page",
		Description: "Refresh the user's browser tabs after you rebuilt the app (proxy mode only). Call once your fix is live so the user sees it immediately.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, okOut, error) {
		if err := h.Reload(ctx); err != nil {
			return nil, okOut{}, err
		}
		return nil, okOut{OK: true}, nil
	})

	addBrowserTools(server, h, &driverPool{outDir: filepath.Dir(*out)})

	fmt.Fprintf(os.Stderr, "eyesore mcp: serving stdio, store %s\n", *out)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
