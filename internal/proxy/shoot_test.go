package proxy_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/proxy"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dispatchNote(t *testing.T, base, payload string) {
	t.Helper()
	resp, err := http.Post(base+"/__eyesore/dispatch", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestDispatchCapturesWhatTheUserWasLookingAt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "notes.json"))

	var mu sync.Mutex
	shots := map[string]string{}
	shoot := func(url, selector string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		shots[selector] = url
		return []byte("png-of-" + selector), nil
	}
	base := startWithShooter(t, appServing("text/html", "<body></body>"), st, shoot)

	dispatchNote(t, base, `[{"id":"es_1","selector":"#hero","note":"fix","url":"http://x/","dispatchedAt":5}]`)

	require.Eventually(t, func() bool {
		b, err := os.ReadFile(notes.ScreenshotPath(dir, "es_1"))
		return err == nil && string(b) == "png-of-#hero"
	}, 3*time.Second, 20*time.Millisecond, "the element PNG must land where the MCP serves it from")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "http://x/", shots["#hero"], "capture must happen on the page the note was made on")
}

func TestACrashedBrowserDoesNotBlockTheNoteItself(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "notes.json"))
	shoot := func(url, selector string) ([]byte, error) { return nil, errors.New("browser gone") }
	base := startWithShooter(t, appServing("text/html", "<body></body>"), st, shoot)

	dispatchNote(t, base, `[{"id":"es_1","selector":"#x","note":"fix","url":"/","dispatchedAt":5}]`)

	all, err := st.Load()
	require.NoError(t, err)
	require.Len(t, all, 1, "notes are the primary artifact; screenshots are best-effort")
	_, err = os.ReadFile(notes.ScreenshotPath(dir, "es_1"))
	assert.Error(t, err)
}

// startWithShooter mirrors startWith but wires a screenshot function.
func startWithShooter(t *testing.T, app http.Handler, st *store.Store, shoot proxy.ShootFunc) string {
	t.Helper()
	backend := httptest.NewServer(app)
	t.Cleanup(backend.Close)
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)
	p := proxy.NewServer(target, st, []byte(overlayJS), 20*time.Millisecond, proxy.WithShooter(shoot))
	t.Cleanup(p.Close)
	front := httptest.NewServer(p)
	t.Cleanup(front.Close)
	return front.URL
}
