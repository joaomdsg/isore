package main

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/joaomdsg/isore/internal/notes"
	"github.com/joaomdsg/isore/internal/proxy"
	"github.com/joaomdsg/isore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestProxy runs a real proxy (as start_proxy would) over st, with a
// fast internal watch-poll so its SSE broadcast fires promptly. subscribeNotes
// requires a reachable proxy; there is no poll-the-store-file fallback.
func startTestProxy(t *testing.T, st *store.Store) string {
	t.Helper()
	target, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)
	px := proxy.NewServer(target, st, []byte("//overlay"), 20*time.Millisecond)
	ts := httptest.NewServer(px)
	t.Cleanup(ts.Close)
	return ts.URL
}

// The persistent listener must keep streaming dispatches until its context is
// canceled, and a canceled context is a clean stop (nil), not an error. If
// subscribeNotes returned after the first batch, it would exit before cancel.
func TestSubscribeStreamsUntilCanceledThenStopsCleanly(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")
	base := startTestProxy(t, store.New(path))
	s := store.New(path)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- subscribeNotes(ctx, path, base, 0) }()

	time.Sleep(30 * time.Millisecond) // let the listener attach
	require.NoError(t, s.Merge([]notes.Note{{ID: "n1", Note: "fix", DispatchedAt: 1}}))
	require.NoError(t, s.Merge([]notes.Note{{ID: "n2", Note: "again", DispatchedAt: 2}}))
	time.Sleep(80 * time.Millisecond) // let both batches stream

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "ctx cancel is a clean stop, not an error")
	case <-time.After(3 * time.Second):
		t.Fatal("subscribeNotes did not return after cancel")
	}
}

func TestSubscribeErrorsWithoutARunningProxy(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")
	err := subscribeNotes(context.Background(), path, "http://127.0.0.1:1", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start_proxy")
}
