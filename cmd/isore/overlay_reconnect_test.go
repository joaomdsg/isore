package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joaomdsg/isore/internal/browser"
	"github.com/joaomdsg/isore/internal/serve"
	"github.com/stretchr/testify/require"
)

func (p *overlayPage) waitBool(expr string, want bool, why string) {
	p.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if p.evalBool(expr) == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	p.t.Fatalf("timeout: %s (want %s == %v)", why, expr, want)
}

// The overlay must recover agent state after its SSE stream dies: a note
// fixed while the proxy was down has to thaw the frozen overlay once the
// proxy is back, without a page reload.
func TestOverlayThawsAfterSSEDrop(t *testing.T) {
	chrome := browser.FindChrome()
	if chrome == "" {
		t.Skip("no chromium/chrome binary")
	}
	st := freshStore(t)
	h := serve.New(st, t.TempDir(), func() int64 { return time.Now().UnixMilli() })
	backend := htmlBackend(t)

	holder := &proxyHolder{}
	base, err := holder.start(backend, "127.0.0.1:0", st, []byte(overlayJS), 50*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = holder.http.Close() })

	d, err := browser.Launch(context.Background(), chrome)
	require.NoError(t, err)
	t.Cleanup(d.Close)
	require.NoError(t, d.Navigate(base+"/"))
	p := &overlayPage{t: t, d: d}
	p.eval(helpers)
	p.waitBool(`!!window.__esSSE&&window.__esSSE.readyState===1`, true, "SSE connected")
	p.enable()

	// Dispatch a note through the real path so the store holds it.
	p.addNote(`{id:'n1',selector:'body',note:'x',url:location.href,createdAt:1,editedAt:1,dispatchedAt:0}`)
	p.eval(`__q('[data-es=dispatch]').click();true`)
	p.waitBool(`window.__es.notes()[0].dispatchedAt>0`, true, "note dispatched")

	require.NoError(t, h.MarkWorking(context.Background(), "n1"))
	p.waitBool(`__q('.es-switch input').disabled`, true, "overlay frozen via SSE")

	// Proxy dies mid-batch; the agent finishes while the tab is deaf.
	addr := strings.TrimPrefix(base, "http://")
	require.NoError(t, holder.http.Close())
	holder.srv.Close()
	require.NoError(t, h.MarkFixed(context.Background(), "n1", "done"))

	// Proxy returns on the same port; the tab must thaw without a reload.
	_, err = holder.start(backend, addr, st, []byte(overlayJS), 50*time.Millisecond)
	require.NoError(t, err)
	p.waitBool(`__q('.es-switch input').disabled`, false, "overlay thawed after reconnect")
}
