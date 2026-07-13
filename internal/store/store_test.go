package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New(filepath.Join(t.TempDir(), "notes.json"))
}

func n(id string, dispatchedAt int64) notes.Note {
	return notes.Note{ID: id, Selector: "#" + id, Note: "note " + id, DispatchedAt: dispatchedAt}
}

func TestAFreshProjectHasAnEmptyStoreNotAnError(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	got, err := s.Load()
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestNotesAccumulateAcrossSeparateDispatches(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("a", 100)}))
	require.NoError(t, s.Merge([]notes.Note{n("b", 200)}))
	got, err := s.Load()
	require.NoError(t, err)
	assert.Len(t, got, 2, "a second dispatch must not wipe earlier notes")
}

func TestAFixSurvivesTheNextDispatch(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("a", 100)}))
	require.NoError(t, s.MarkFixed("a", 500, ""))
	require.NoError(t, s.Merge([]notes.Note{n("b", 600)}))
	got, err := s.Load()
	require.NoError(t, err)
	byID := map[string]notes.Note{}
	for _, g := range got {
		byID[g.ID] = g
	}
	assert.Equal(t, int64(500), byID["a"].FixedAt)
}

func TestHarnessAndMCPProcessesSeeEachOthersWritesThroughTheFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")
	harness := store.New(path)
	mcp := store.New(path)

	require.NoError(t, harness.Merge([]notes.Note{n("a", 100)}))
	got, err := mcp.Load()
	require.NoError(t, err)
	require.Len(t, got, 1, "a separate store instance must read dispatches from disk")

	require.NoError(t, mcp.MarkFixed("a", 500, ""))
	got, err = harness.Load()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, int64(500), got[0].FixedAt, "fixes must round-trip through the file too")
}

func TestMarkingAnUnknownNoteFailsWithoutTouchingTheStore(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("a", 100)}))
	assert.Error(t, s.MarkFixed("ghost", 500, ""))
	got, err := s.Load()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Zero(t, got[0].FixedAt)
}

func TestACorruptStoreFileSurfacesInsteadOfBeingSilentlyOverwritten(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")
	require.NoError(t, os.WriteFile(path, []byte("{corrupt"), 0o644))
	s := store.New(path)
	_, err := s.Load()
	assert.Error(t, err)
	assert.Error(t, s.Merge([]notes.Note{n("a", 100)}),
		"merging over a corrupt store would destroy whatever the user had")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "{corrupt", string(raw), "the evidence must be left in place for the user")
}

func TestAnEmptyFileIsAnEmptyStoreNotCorruption(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")
	require.NoError(t, os.WriteFile(path, nil, 0o644))
	s := store.New(path)
	got, err := s.Load()
	require.NoError(t, err, "a zero-byte file (touch, truncation) holds no notes but is not corrupt")
	assert.Empty(t, got)
}

func TestMarkFixedOnAFreshProjectReportsUnknownNote(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	assert.Error(t, s.MarkFixed("ghost", 500, ""))
}

func TestAwaitWakesWhenANewNoteIsDispatched(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("old", 100)}))

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = s.Merge([]notes.Note{n("fresh", 999)})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	got, err := s.Await(ctx, 100, 5*time.Millisecond)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "fresh", got[0].ID)
}

func TestAwaitReturnsImmediatelyWhenFreshNotesAlreadyExist(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("fresh", 300)}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	got, err := s.Await(ctx, 200, time.Second)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Less(t, time.Since(start), 500*time.Millisecond,
		"must check before the first poll sleep")
}

func TestAwaitTimesOutEmptyHandedWithoutError(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	got, err := s.Await(ctx, 0, 5*time.Millisecond)
	require.NoError(t, err, "an empty wait is a normal outcome for an agent, not a failure")
	assert.Empty(t, got)
}
