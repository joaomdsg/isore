package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/joaomdsg/isore/internal/serve"
	"github.com/joaomdsg/isore/internal/store"
)

// runSubscribe is the CLI listener for dispatched annotations. It opens ONE
// connection to the proxy's SSE stream and prints one JSON batch per dispatch
// as it arrives, staying connected across dispatches and never exiting on its
// own. Run it once as a single persistent background listener (under the
// harness Monitor) so the agent is notified of every dispatch without the
// channel ever closing.
func runSubscribe(args []string) error {
	fs := flag.NewFlagSet("isore subscribe", flag.ExitOnError)
	out := fs.String("out", "isore-out/notes.json", "notes store shared with the harness")
	proxyPort := fs.Int("proxyPort", 4400, "port of the running proxy (started via start_proxy) — required, since this waits on the proxy's SSE stream")
	sinceMs := fs.Int64("sinceMs", 0, "only notes dispatched after this unix-ms timestamp; 0 checkpoints at the latest dispatch already in the store")
	if err := fs.Parse(args); err != nil {
		return err
	}
	proxyBase := fmt.Sprintf("http://127.0.0.1:%d", *proxyPort)
	return subscribeNotes(context.Background(), *out, proxyBase, *sinceMs)
}

// subscribeNotes holds ONE SSE connection to the proxy open and prints one JSON
// batch per dispatch as it arrives, never exiting between dispatches. A clean
// stream drop (a proxy blip) is retried with the checkpoint advanced past
// everything already printed, so no dispatch is delivered twice; a hard error
// (the proxy is gone) returns so the agent relaunches the listener after a
// fresh start_proxy. A canceled ctx is a clean stop, not an error.
func subscribeNotes(ctx context.Context, outPath, proxyBase string, sinceMs int64) error {
	h := serve.New(store.New(outPath), filepath.Dir(outPath), func() int64 {
		return time.Now().UnixMilli()
	})
	h.SetProxyBase(proxyBase)
	enc := json.NewEncoder(os.Stdout)
	emit := func(ns []serve.NoteView) error {
		if ns == nil {
			ns = []serve.NoteView{}
		}
		return enc.Encode(ns) // os.Stdout is unbuffered: each batch reaches the listener at once
	}
	since := sinceMs
	for {
		next, err := h.Follow(ctx, since, emit)
		if err != nil {
			return err
		}
		since = next
		if ctx.Err() != nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond) // brief backoff before reconnecting on a clean drop
	}
}
