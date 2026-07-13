package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/joaomdsg/eyesore/internal/proxy"
	"github.com/joaomdsg/eyesore/internal/store"
)

func runProxy(args []string) error {
	fs := flag.NewFlagSet("eyesore proxy", flag.ExitOnError)
	target := fs.String("url", "http://127.0.0.1:3000", "dev server to proxy")
	listen := fs.String("listen", "127.0.0.1:4400", "address to serve the annotated app on")
	out := fs.String("out", "eyesore-out/notes.json", "notes store shared with the MCP server")
	if err := fs.Parse(args); err != nil {
		return err
	}
	t, err := url.Parse(*target)
	if err != nil {
		return fmt.Errorf("bad -url: %w", err)
	}

	pool := &driverPool{outDir: filepath.Dir(*out)}
	var shootMu sync.Mutex // one driver, one tab: navigations must not interleave
	shoot := func(pageURL, selector string) ([]byte, error) {
		shootMu.Lock()
		defer shootMu.Unlock()
		d, err := pool.get(context.Background())
		if err != nil {
			return nil, err
		}
		if err := d.Navigate(pageURL); err != nil {
			return nil, err
		}
		return d.Screenshot(selector)
	}
	p := proxy.NewServer(t, store.New(*out), []byte(overlayJS), 300*time.Millisecond, proxy.WithShooter(shoot))
	defer p.Close()
	fmt.Fprintf(os.Stderr, "eyesore proxy: browse http://%s (annotating %s), store %s\n", *listen, *target, *out)
	return http.ListenAndServe(*listen, p)
}
