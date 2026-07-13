package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Driver drives one Chromium — either the user's visible harness browser
// (Attach) or an owned headless instance (Launch). Not safe for concurrent
// tool calls by design: MCP agents call tools sequentially.
type Driver struct {
	ctx     context.Context
	cancels []context.CancelFunc
	Console *Ring
}

// Attach connects to a running browser's CDP endpoint (harness mode).
func Attach(parent context.Context, debugURL string) (*Driver, error) {
	alloc, cancelAlloc := chromedp.NewRemoteAllocator(parent, debugURL)
	ctx, cancelCtx := chromedp.NewContext(alloc)
	d := &Driver{ctx: ctx, cancels: []context.CancelFunc{cancelCtx, cancelAlloc}, Console: NewRing(256)}
	d.listen()
	// verify the target is actually there
	if err := chromedp.Run(ctx); err != nil {
		d.Close()
		return nil, fmt.Errorf("attach %s: %w", debugURL, err)
	}
	return d, nil
}

// Launch starts an owned headless browser (proxy mode). chromePath "" means
// auto-detect.
func Launch(parent context.Context, chromePath string) (*Driver, error) {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	alloc, cancelAlloc := chromedp.NewExecAllocator(parent, opts...)
	ctx, cancelCtx := chromedp.NewContext(alloc)
	d := &Driver{ctx: ctx, cancels: []context.CancelFunc{cancelCtx, cancelAlloc}, Console: NewRing(256)}
	d.listen()
	if err := chromedp.Run(ctx); err != nil {
		d.Close()
		return nil, fmt.Errorf("launch headless browser: %w", err)
	}
	return d, nil
}

func (d *Driver) listen() {
	chromedp.ListenTarget(d.ctx, func(ev interface{}) {
		if e, ok := ev.(*runtime.EventConsoleAPICalled); ok {
			line := string(e.Type) + ":"
			for _, a := range e.Args {
				if len(a.Value) > 0 {
					line += " " + string(a.Value)
				} else if a.Description != "" {
					line += " " + a.Description
				}
			}
			d.Console.Add(line)
		}
	})
}

func (d *Driver) Close() {
	for _, c := range d.cancels {
		c()
	}
}

func (d *Driver) run(timeout time.Duration, actions ...chromedp.Action) error {
	ctx, cancel := context.WithTimeout(d.ctx, timeout)
	defer cancel()
	return chromedp.Run(ctx, actions...)
}

// Navigate loads a URL in the driven tab.
func (d *Driver) Navigate(url string) error {
	return d.run(30*time.Second, chromedp.Navigate(url))
}

// Screenshot captures the element at selector, or the viewport if empty.
func (d *Driver) Screenshot(selector string) ([]byte, error) {
	var buf []byte
	var act chromedp.Action
	if selector == "" {
		act = chromedp.CaptureScreenshot(&buf)
	} else {
		act = chromedp.Screenshot(selector, &buf, chromedp.ByQuery)
	}
	if err := d.run(15*time.Second, act); err != nil {
		return nil, err
	}
	return buf, nil
}

// Eval runs a JS expression in the page and returns its JSON value.
func (d *Driver) Eval(expr string) (json.RawMessage, error) {
	var out json.RawMessage
	if err := d.run(15*time.Second, chromedp.Evaluate(expr, &out)); err != nil {
		return nil, err
	}
	return out, nil
}

// HTML returns the outer HTML of the element at selector ("html" for the page).
func (d *Driver) HTML(selector string) (string, error) {
	if selector == "" {
		selector = "html"
	}
	var out string
	if err := d.run(15*time.Second, chromedp.OuterHTML(selector, &out, chromedp.ByQuery)); err != nil {
		return "", err
	}
	return out, nil
}

// FindChrome returns a usable browser binary path, or "".
func FindChrome() string {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}
