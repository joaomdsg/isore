// Command eyesore opens a running web app in a NON-headless Chromium
// via chromedp and injects a toggleable annotation overlay. Hover highlights the
// element under the cursor, click opens a compact inline note input at the
// element location, and a floating "Dispatch" button ships edited notes back to
// this process which writes them (and element screenshots) to -out. Notes
// persist across page navigations via localStorage. Ctrl-Shift-N toggles.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/joaomdsg/eyesore/internal/browser"
	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/store"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp":
			if err := runMCP(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "eyesore mcp:", err)
				os.Exit(1)
			}
			return
		case "proxy":
			if err := runProxy(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "eyesore proxy:", err)
				os.Exit(1)
			}
			return
		}
	}
	url := flag.String("url", "http://127.0.0.1:3000/", "app URL to annotate")
	out := flag.String("out", "eyesore-out/notes.json", "dispatched notes output")
	chrome := flag.String("chrome", "", "browser executable (default: auto-detect)")
	debugPort := flag.String("debug-port", "9222", "CDP port exposed to `eyesore mcp` browser tools")
	flag.Parse()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("remote-debugging-port", *debugPort),
	)
	if *chrome != "" {
		opts = append(opts, chromedp.ExecPath(*chrome))
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	outDir := filepath.Dir(*out)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch be := ev.(type) {
		case *runtime.EventBindingCalled:
			fmt.Printf("binding: %s payload_len=%d\n", be.Name, len(be.Payload))
			handleBinding(ctx, be, *out, outDir)
		case *runtime.EventConsoleAPICalled:
			// forward browser console.log to stdout for debugging
			for _, a := range be.Args {
				if a.Type == "string" {
					fmt.Printf("browser: %s\n", a.Value)
				}
			}
		}
	})

	var ready bool
	if err := chromedp.Run(ctx,
		runtime.AddBinding("esDispatch"),
		runtime.AddBinding("esDelete"),
		runtime.AddBinding("esEdit"),
		runtime.AddBinding("esToggle"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(overlayJS).Do(ctx)
			return err
		}),
		chromedp.Navigate(*url),
		chromedp.Evaluate(overlayJS, &ready),
	); err != nil {
		fmt.Fprintln(os.Stderr, "eyesore run:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outDir, 0o755); err == nil {
		endpoint := filepath.Join(outDir, "browser.json")
		if werr := browser.WriteEndpoint(endpoint, "http://127.0.0.1:"+*debugPort); werr == nil {
			defer os.Remove(endpoint)
		}
	}
	fmt.Printf("eyesore ready on %s — Ctrl-Shift-N to toggle, click elements to annotate, Dispatch to ship. Ctrl-C to quit.\n", *url)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case <-ctx.Done():
	case <-sig:
	case <-time.After(60 * time.Minute):
	}
}

func handleBinding(ctx context.Context, be *runtime.EventBindingCalled, outPath, outDir string) {
	switch be.Name {
	case "esDispatch":
		dispatched, ok := notes.Parse([]byte(be.Payload))
		if !ok || len(dispatched) == 0 {
			return
		}
		captureScreenshots(ctx, dispatched, outDir)
		if err := store.New(outPath).Merge(dispatched); err != nil {
			fmt.Fprintln(os.Stderr, "store merge:", err)
			return
		}
		data, _ := json.MarshalIndent(dispatched, "", "  ")
		fmt.Printf("=== DISPATCHED %d NOTE(S) ===\n", len(dispatched))
		fmt.Println(string(data))
		fmt.Println("=== END NOTES ===")

	case "esDelete":
		e, ok := parseDeleteEvent([]byte(be.Payload))
		if ok {
			fmt.Printf("note deleted: %s\n", e.ID)
		}

	case "esEdit":
		e, ok := parseEditEvent([]byte(be.Payload))
		if ok {
			fmt.Printf("note edited: %s\n", e.ID)
		}

	case "esToggle":
		fmt.Printf("harness toggled: %s\n", be.Payload)
	}
}

func captureScreenshots(ctx context.Context, ns []notes.Note, outDir string) {
	ssDir := filepath.Join(outDir, "screenshots")
	if err := os.MkdirAll(ssDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "screenshot dir: %v\n", err)
		return
	}
	for i := range ns {
		var buf []byte
		if err := chromedp.Run(ctx,
			chromedp.Screenshot(ns[i].Selector, &buf, chromedp.ByQuery),
		); err != nil {
			fmt.Fprintf(os.Stderr, "screenshot %s: %v\n", ns[i].ID, err)
			continue
		}
		path := filepath.Join(ssDir, ns[i].ID+".png")
		if err := os.WriteFile(path, buf, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "screenshot write %s: %v\n", path, err)
		}
	}
}
