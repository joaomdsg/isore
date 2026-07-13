package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/joaomdsg/eyesore/internal/browser"
	"github.com/joaomdsg/eyesore/internal/serve"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// driverPool lazily acquires the browser: attach to the harness's visible
// Chromium when its handshake file exists, otherwise launch owned headless.
type driverPool struct {
	mu     sync.Mutex
	d      *browser.Driver
	outDir string
}

func (p *driverPool) get(ctx context.Context) (*browser.Driver, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.d != nil {
		return p.d, nil
	}
	var d *browser.Driver
	if debugURL, err := browser.ReadEndpoint(filepath.Join(p.outDir, "browser.json")); err == nil {
		if d, err = browser.Attach(context.Background(), debugURL); err != nil {
			return nil, err
		}
	} else {
		chrome := browser.FindChrome()
		if chrome == "" {
			return nil, fmt.Errorf("no attached browser and no chromium/chrome binary found")
		}
		var lerr error
		if d, lerr = browser.Launch(context.Background(), chrome); lerr != nil {
			return nil, lerr
		}
	}
	p.d = d
	return d, nil
}

type screenshotIn struct {
	ID string `json:"id" jsonschema:"note id whose element screenshot to fetch"`
}

type browserShotIn struct {
	Selector string `json:"selector,omitempty" jsonschema:"CSS selector to capture; empty captures the viewport"`
	URL      string `json:"url,omitempty" jsonschema:"navigate here first; empty stays on the current page"`
}

type evalIn struct {
	Expression string `json:"expression" jsonschema:"JavaScript expression evaluated in the page; result returned as JSON"`
}

type htmlIn struct {
	Selector string `json:"selector,omitempty" jsonschema:"CSS selector; empty returns the whole document"`
}

type navigateIn struct {
	URL string `json:"url" jsonschema:"absolute URL to load in the driven tab"`
}

type consoleIn struct {
	Limit int `json:"limit,omitempty" jsonschema:"newest messages to return; default 50"`
}

func imageResult(png []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{Data: png, MIMEType: "image/png"},
	}}
}

func addBrowserTools(server *mcp.Server, h *serve.Handlers, pool *driverPool) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_screenshot",
		Description: "LOOK at the element the user annotated: returns the PNG captured when the note was dispatched. Always view it before changing code.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in screenshotIn) (*mcp.CallToolResult, any, error) {
		png, err := h.Screenshot(ctx, in.ID)
		if err != nil {
			return nil, nil, err
		}
		return imageResult(png), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_screenshot",
		Description: "Capture the live page (or one element) right now — use it to verify your fix actually rendered.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in browserShotIn) (*mcp.CallToolResult, any, error) {
		d, err := pool.get(ctx)
		if err != nil {
			return nil, nil, err
		}
		if in.URL != "" {
			if err := d.Navigate(in.URL); err != nil {
				return nil, nil, err
			}
		}
		png, err := d.Screenshot(in.Selector)
		if err != nil {
			return nil, nil, err
		}
		return imageResult(png), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_eval",
		Description: "Run a JavaScript expression in the live page and get its JSON result — inspect state, computed styles, anything the DevTools console could.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in evalIn) (*mcp.CallToolResult, any, error) {
		d, err := pool.get(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := d.Eval(in.Expression)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(out)}}}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_html",
		Description: "Read the live outer HTML of an element (or the whole document) from the driven browser.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in htmlIn) (*mcp.CallToolResult, any, error) {
		d, err := pool.get(ctx)
		if err != nil {
			return nil, nil, err
		}
		out, err := d.HTML(in.Selector)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: out}}}, nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_navigate",
		Description: "Point the driven browser at a URL (e.g. the page a note was made on).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in navigateIn) (*mcp.CallToolResult, okOut, error) {
		d, err := pool.get(ctx)
		if err != nil {
			return nil, okOut{}, err
		}
		if err := d.Navigate(in.URL); err != nil {
			return nil, okOut{}, err
		}
		return nil, okOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "browser_console",
		Description: "Read recent console output (logs, warnings, errors) from the driven browser — captured from the moment the browser was attached.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in consoleIn) (*mcp.CallToolResult, any, error) {
		d, err := pool.get(ctx)
		if err != nil {
			return nil, nil, err
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 50
		}
		lines := d.Console.Last(limit)
		text := ""
		for _, l := range lines {
			text += l + "\n"
		}
		if text == "" {
			text = "(console is quiet)"
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
	})
}
