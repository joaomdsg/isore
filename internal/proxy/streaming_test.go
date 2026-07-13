package proxy_test

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/proxy"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheAppsOwnEventStreamsFlowWithoutBuffering(t *testing.T) {
	t.Parallel()
	base, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "data: hmr-update\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // stream stays open, like a real HMR channel
	}))

	headerBound := &http.Client{Transport: &http.Transport{ResponseHeaderTimeout: 2 * time.Second}}
	resp, err := headerBound.Get(base + "/hmr")
	require.NoError(t, err, "headers must arrive while the stream is open, not after EOF")
	defer resp.Body.Close()

	got := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(resp.Body).ReadString('\n')
		got <- line
	}()
	select {
	case line := <-got:
		assert.Equal(t, "data: hmr-update\n", line,
			"first bytes must reach the browser while the stream is still open")
	case <-time.After(2 * time.Second):
		t.Fatal("stream was buffered: dev-server HMR would be dead through the proxy")
	}
}

func TestWebSocketUpgradesPassThroughSoHMRSocketsWork(t *testing.T) {
	t.Parallel()
	base, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: raw\r\nConnection: Upgrade\r\n\r\nhello-socket")
	}))

	u, err := url.Parse(base)
	require.NoError(t, err)
	conn, err := net.DialTimeout("tcp", u.Host, 2*time.Second)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))

	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s\r\nUpgrade: raw\r\nConnection: Upgrade\r\n\r\n", u.Host)
	raw, err := io.ReadAll(conn)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "101 Switching Protocols")
	assert.Contains(t, string(raw), "hello-socket",
		"post-upgrade bytes must tunnel through or HMR websockets die")
}

func TestABackendThatGzipsAnywayIsLeftAloneRatherThanCorrupted(t *testing.T) {
	t.Parallel()
	gzipped := "\x1f\x8b-fake-gzip-bytes-</body>"
	base, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "gzip")
		io.WriteString(w, gzipped)
	}))

	req, err := http.NewRequest(http.MethodGet, base+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultTransport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, gzipped, string(b),
		"splicing a script tag into compressed bytes would serve garbage")
	assert.NotContains(t, string(b), "overlay.js")
}

func TestConcurrentShutdownPathsCannotPanicTheProxy(t *testing.T) {
	t.Parallel()
	target, err := url.Parse("http://127.0.0.1:0")
	require.NoError(t, err)
	st := store.New(filepath.Join(t.TempDir(), "notes.json"))
	p := proxy.NewServer(target, st, nil, time.Hour)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); p.Close() }()
	}
	wg.Wait()
}
