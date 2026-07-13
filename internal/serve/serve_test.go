package serve_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/serve"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) (*store.Store, *serve.Handlers, string) {
	t.Helper()
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "notes.json"))
	h := serve.New(s, dir, func() int64 { return 7777 })
	return s, h, dir
}

func dispatch(t *testing.T, s *store.Store, ns ...notes.Note) {
	t.Helper()
	require.NoError(t, s.Merge(ns))
}

func n(id string, dispatchedAt, fixedAt int64) notes.Note {
	return notes.Note{ID: id, Selector: "#" + id, Label: "label " + id, Note: "note " + id,
		URL: "http://localhost:3000/", DispatchedAt: dispatchedAt, FixedAt: fixedAt}
}

func TestAgentsSeeOnlyUnfixedNotesWithTheirFullContext(t *testing.T) {
	t.Parallel()
	s, h, dir := setup(t)
	dispatch(t, s, n("a", 100, 0), n("b", 100, 500))

	got, err := h.ListNotes(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 1, "fixed notes are done — agents must not re-fix them")
	assert.Equal(t, "a", got[0].ID)
	assert.Equal(t, "#a", got[0].Selector)
	assert.Equal(t, "label a", got[0].Label)
	assert.Equal(t, "note a", got[0].Note)
	assert.Equal(t, "http://localhost:3000/", got[0].URL)
	assert.Equal(t, int64(100), got[0].DispatchedAt)
	assert.Equal(t, notes.ScreenshotPath(dir, "a"), got[0].Screenshot,
		"screenshot path must point into the out dir next to the store")
}

func TestMarkFixedStampsWithTheServersClockAndPersists(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("a", 100, 0))

	require.NoError(t, h.MarkFixed(context.Background(), "a", ""))

	all, err := s.Load()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, int64(7777), all[0].FixedAt)
}

func TestMarkFixedOnUnknownIDTellsTheAgentInsteadOfSilentlySucceeding(t *testing.T) {
	t.Parallel()
	_, h, _ := setup(t)
	assert.Error(t, h.MarkFixed(context.Background(), "ghost", ""))
}

func TestAwaitCheckpointsAtTheLatestKnownDispatchSoOldNotesDoNotRetrigger(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("early", 100, 0), n("late", 400, 0))

	mergeErr := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		mergeErr <- s.Merge([]notes.Note{n("between", 250, 0)})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	got, err := h.Await(ctx, 0)
	require.NoError(t, err)
	require.NoError(t, <-mergeErr)
	assert.Empty(t, got,
		"checkpoint is the MAX known dispatch time; anything at or below it is old news")
}

func TestAwaitDeliversNotesDispatchedWhileTheAgentWaits(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("old", 100, 0))

	mergeErr := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		mergeErr <- s.Merge([]notes.Note{n("fresh", 999, 0)})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := h.Await(ctx, 0)
	require.NoError(t, err)
	require.NoError(t, <-mergeErr)
	require.Len(t, got, 1)
	assert.Equal(t, "fresh", got[0].ID)
	assert.NotEmpty(t, got[0].Screenshot)
}

func TestAwaitOnAnEmptyStoreCatchesTheVeryFirstDispatch(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)

	mergeErr := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		mergeErr <- s.Merge([]notes.Note{n("first", 1, 0)})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := h.Await(ctx, 0)
	require.NoError(t, err)
	require.NoError(t, <-mergeErr)
	require.Len(t, got, 1)
	assert.Equal(t, "first", got[0].ID,
		"an empty store checkpoints at zero, so even a tiny dispatch time counts as fresh")
}

func TestAwaitWithExplicitCheckpointReplaysNotesTheAgentMissed(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("missed", 300, 0))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := h.Await(ctx, 200)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "missed", got[0].ID)
}

func TestACorruptStoreSurfacesToTheAgentInsteadOfLookingEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.json")
	require.NoError(t, os.WriteFile(path, []byte("{corrupt"), 0o644))
	h := serve.New(store.New(path), dir, func() int64 { return 7777 })

	_, err := h.ListNotes(context.Background())
	assert.Error(t, err, "an empty answer would hide real user notes from the agent")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = h.Await(ctx, 0)
	assert.Error(t, err)

	_, err = h.Await(ctx, 1)
	assert.Error(t, err, "corruption while long-polling must also surface")
}
