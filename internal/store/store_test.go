package store_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/joaomdsg/isore/internal/notes"
	"github.com/joaomdsg/isore/internal/store"
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

func TestDeletePersistsAcrossReload(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("a", 100), n("b", 100)}))
	require.NoError(t, s.Delete("a"))
	got, err := s.Load()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "b", got[0].ID)
}

func TestAddCommentPersistsAcrossReload(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	require.NoError(t, s.Merge([]notes.Note{n("a", 100)}))
	require.NoError(t, s.AddComment("a", "also fix the footer", 500))
	got, err := s.Load()
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Comments, 1)
	assert.Equal(t, "also fix the footer", got[0].Comments[0].Text)
	assert.Equal(t, "note a", got[0].Note, "comment must not overwrite the original note text")
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

func TestConcurrentWritersLoseNothing(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "notes.json")

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// each writer is its own instance, like separate processes
			_ = store.New(path).Merge([]notes.Note{n(fmt.Sprintf("es_%d", i), int64(i+1))})
		}(i)
	}
	wg.Wait()

	got, err := store.New(path).Load()
	require.NoError(t, err)
	assert.Len(t, got, 20, "read-modify-write must not drop concurrent dispatches")
}

func TestConcurrentFixesAllStick(t *testing.T) {
	t.Parallel()
	// the real incident: three mark_fixed calls served concurrently by the
	// MCP SDK — every one reported success, only the last write survived
	path := filepath.Join(t.TempDir(), "notes.json")
	s := store.New(path)
	require.NoError(t, s.Merge([]notes.Note{n("a", 1), n("b", 2), n("c", 3)}))

	var wg sync.WaitGroup
	for _, id := range []string{"a", "b", "c"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			assert.NoError(t, store.New(path).MarkFixed(id, 500, "done"))
		}(id)
	}
	wg.Wait()

	got, err := s.Load()
	require.NoError(t, err)
	fixed := 0
	for _, g := range got {
		if g.FixedAt != 0 {
			fixed++
		}
	}
	assert.Equal(t, 3, fixed, "a fix that reported success must never be silently undone")
}
