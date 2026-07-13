package proxy_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/proxy"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const overlayJS = "/*eyesore overlay*/ console.log('__eyesore');"

// start wires app -> eyesore proxy -> test client around a fresh store.
func start(t *testing.T, app http.Handler) (base string, st *store.Store) {
	t.Helper()
	st = store.New(filepath.Join(t.TempDir(), "notes.json"))
	return startWith(t, app, st), st
}

// startWith lets a test seed the store BEFORE the proxy takes its baseline
// snapshot — pre-existing notes are not "changes" and must not broadcast.
func startWith(t *testing.T, app http.Handler, st *store.Store) (base string) {
	t.Helper()
	backend := httptest.NewServer(app)
	t.Cleanup(backend.Close)
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)
	p := proxy.NewServer(target, st, []byte(overlayJS), 20*time.Millisecond)
	t.Cleanup(p.Close)

	front := httptest.NewServer(p)
	t.Cleanup(front.Close)
	return front.URL
}

func appServing(contentType, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		io.WriteString(w, body)
	})
}

func get(t *testing.T, url string) (int, http.Header, string) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(b)
}

func TestUsersSeeTheirAppWithTheOverlayPlantedIn(t *testing.T) {
	t.Parallel()
	base, _ := start(t, appServing("text/html", "<html><body><h1>app</h1></body></html>"))
	code, _, body := get(t, base+"/")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "<h1>app</h1>", "the app itself must be intact")
	assert.Contains(t, body, `<script src="/__eyesore/overlay.js"></script>`)
}

func TestAssetsFlowThroughByteForByte(t *testing.T) {
	t.Parallel()
	payload := `{"users":["</body>"]}`
	base, _ := start(t, appServing("application/json", payload))
	code, _, body := get(t, base+"/api/users")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, payload, body)
}

func TestCompressionIsNegotiatedAwaySoInjectionSeesPlainHTML(t *testing.T) {
	t.Parallel()
	seen := make(chan string, 1)
	base, _ := start(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<body></body>")
	}))
	req, err := http.NewRequest(http.MethodGet, base+"/", nil)
	require.NoError(t, err)
	req.Header.Set("Accept-Encoding", "gzip, br")
	resp, err := http.DefaultTransport.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Empty(t, <-seen, "proxy must strip Accept-Encoding or gzip bodies would be spliced")
	b, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(b), "overlay.js")
}

func TestTheOverlayScriptIsServedFromTheMagicPath(t *testing.T) {
	t.Parallel()
	base, _ := start(t, appServing("text/html", "<body></body>"))
	code, header, body := get(t, base+"/__eyesore/overlay.js")
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, header.Get("Content-Type"), "javascript")
	assert.Equal(t, overlayJS, body)
}

func TestDispatchedNotesLandInTheSharedStore(t *testing.T) {
	t.Parallel()
	base, st := start(t, appServing("text/html", "<body></body>"))
	payload := `[{"id":"es_1","selector":"#x","note":"fix me","url":"/","dispatchedAt":5}]`
	resp, err := http.Post(base+"/__eyesore/dispatch", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	all, err := st.Load()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "fix me", all[0].Note)
}

func TestGarbageDispatchesAreRejectedWithoutPoisoningTheStore(t *testing.T) {
	t.Parallel()
	base, st := start(t, appServing("text/html", "<body></body>"))
	resp, err := http.Post(base+"/__eyesore/dispatch", "application/json", strings.NewReader("not json"))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	all, err := st.Load()
	require.NoError(t, err)
	assert.Empty(t, all)
}

// sse subscribes and returns a channel of "event|data" strings.
func sse(t *testing.T, base string) <-chan string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/__eyesore/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	events := make(chan string, 16)
	go func() {
		defer close(events)
		sc := bufio.NewScanner(resp.Body)
		var ev, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event:"):
				ev = strings.TrimSpace(line[6:])
			case strings.HasPrefix(line, "data:"):
				data = strings.TrimSpace(line[5:])
			case line == "" && ev != "":
				events <- ev + "|" + data
				ev, data = "", ""
			}
		}
	}()
	return events
}

func waitFor(t *testing.T, events <-chan string, want string) string {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed before %q", want)
			}
			if strings.HasPrefix(e, want+"|") {
				return e
			}
		case <-deadline:
			t.Fatalf("no %q event within 3s", want)
		}
	}
}

func TestAReloadRequestReachesEveryOpenTab(t *testing.T) {
	t.Parallel()
	base, _ := start(t, appServing("text/html", "<body></body>"))
	tab1 := sse(t, base)
	tab2 := sse(t, base)

	resp, err := http.Post(base+"/__eyesore/reload", "", nil)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	waitFor(t, tab1, "reload")
	waitFor(t, tab2, "reload")
}

func TestAgentActivityWrittenByAnotherProcessShowsUpInTheOverlay(t *testing.T) {
	t.Parallel()
	st := store.New(filepath.Join(t.TempDir(), "notes.json"))
	require.NoError(t, st.Merge([]notes.Note{{ID: "es_1", Selector: "#x", Note: "fix", DispatchedAt: 5}}))
	base := startWith(t, appServing("text/html", "<body></body>"), st)
	events := sse(t, base)

	// the MCP process writes through its own store instance
	require.NoError(t, st.MarkWorking("es_1"))
	got := waitFor(t, events, "notes")
	assert.Contains(t, got, `"agentStatus":"working"`)

	require.NoError(t, st.MarkFixed("es_1", 500, "made it green"))
	got = waitFor(t, events, "notes")
	assert.Contains(t, got, `"agentSummary":"made it green"`)
}

func TestQuietStoresSendNoNoteEventsSoTabsDoNotChurn(t *testing.T) {
	t.Parallel()
	st := store.New(filepath.Join(t.TempDir(), "notes.json"))
	require.NoError(t, st.Merge([]notes.Note{{ID: "es_1", Note: "fix", DispatchedAt: 5}}))
	base := startWith(t, appServing("text/html", "<body></body>"), st)
	events := sse(t, base)

	// no initial paint, no churn: pre-existing state is the overlay's own
	// localStorage business — the stream carries only changes after subscribe
	select {
	case e := <-events:
		t.Fatalf("store never changed but got event %q", e)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestASecondDispatchThroughTheProxyKeepsTheFirst(t *testing.T) {
	t.Parallel()
	base, st := start(t, appServing("text/html", "<body></body>"))
	for _, p := range []string{
		`[{"id":"es_1","note":"one","dispatchedAt":5}]`,
		`[{"id":"es_2","note":"two","dispatchedAt":6}]`,
	} {
		resp, err := http.Post(base+"/__eyesore/dispatch", "application/json", strings.NewReader(p))
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
	}
	all, err := st.Load()
	require.NoError(t, err)
	assert.Len(t, all, 2, "the dispatch endpoint must merge, never overwrite")
}
