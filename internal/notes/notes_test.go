package notes_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func n(id string, dispatchedAt, fixedAt int64) notes.Note {
	return notes.Note{ID: id, Selector: "#" + id, Note: "note " + id,
		DispatchedAt: dispatchedAt, FixedAt: fixedAt}
}

func TestDispatchMergePreservesEarlierNotesAndTheirFixedStatus(t *testing.T) {
	t.Parallel()
	existing := []notes.Note{n("a", 100, 150), n("b", 100, 0)}
	edited := n("b", 200, 0)
	edited.Note = "updated text"
	incoming := []notes.Note{edited, n("c", 200, 0)}

	merged := notes.Merge(existing, incoming)

	require.Len(t, merged, 3)
	byID := map[string]notes.Note{}
	for _, m := range merged {
		byID[m.ID] = m
	}
	assert.Equal(t, int64(150), byID["a"].FixedAt, "untouched note must keep its fixed status")
	assert.Equal(t, "updated text", byID["b"].Note, "re-dispatched note must carry the new text")
	assert.Contains(t, byID, "c", "newly dispatched note must be added")
}

func TestRedispatchingAFixedNoteReopensIt(t *testing.T) {
	t.Parallel()
	existing := []notes.Note{n("a", 100, 150)}
	merged := notes.Merge(existing, []notes.Note{n("a", 200, 0)})
	require.Len(t, merged, 1)
	assert.Zero(t, merged[0].FixedAt, "user re-dispatched the note, so it is pending again")
}

func TestPendingExcludesNotesAlreadyFixed(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 150), n("b", 100, 0), n("c", 200, 0)}
	pending := notes.Pending(all)
	require.Len(t, pending, 2)
	for _, p := range pending {
		assert.Zero(t, p.FixedAt)
	}
}

func TestMarkFixedStampsOnlyTheTargetNote(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0), n("b", 100, 0)}
	updated, err := notes.MarkFixed(all, "a", 500, "")
	require.NoError(t, err)
	require.Len(t, updated, 2)
	byID := map[string]notes.Note{}
	for _, u := range updated {
		byID[u.ID] = u
	}
	assert.Equal(t, int64(500), byID["a"].FixedAt)
	assert.Zero(t, byID["b"].FixedAt)
	assert.Zero(t, all[0].FixedAt, "caller's slice must not be mutated")
}

func TestMarkFixedOnUnknownIDFailsSoAgentsCannotSilentlyMissTheTarget(t *testing.T) {
	t.Parallel()
	_, err := notes.MarkFixed([]notes.Note{n("a", 100, 0)}, "nope", 500, "")
	assert.Error(t, err)
}

func TestMarkFixedTwiceKeepsTheOriginalFixTime(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0)}
	once, err := notes.MarkFixed(all, "a", 500, "")
	require.NoError(t, err)
	twice, err := notes.MarkFixed(once, "a", 900, "")
	require.NoError(t, err)
	require.Len(t, twice, 1)
	assert.Equal(t, int64(500), twice[0].FixedAt)
}

func TestFirstDispatchIntoAnEmptyStoreKeepsEveryNote(t *testing.T) {
	t.Parallel()
	merged := notes.Merge(nil, []notes.Note{n("a", 100, 0), n("b", 100, 0)})
	assert.Len(t, merged, 2)
}

func TestEmptyDispatchLeavesTheStoreUntouched(t *testing.T) {
	t.Parallel()
	existing := []notes.Note{n("a", 100, 150)}
	merged := notes.Merge(existing, nil)
	require.Len(t, merged, 1)
	assert.Equal(t, int64(150), merged[0].FixedAt)
}

func TestNoteDispatchedExactlyAtTheCheckpointIsNotReRead(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("edge", 200, 0)}
	assert.Empty(t, notes.NewSince(all, 200),
		"checkpoint is the last seen dispatch time; equal means already seen")
}

func TestNewSinceSurfacesOnlyNotesDispatchedAfterTheCheckpoint(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("old", 100, 0), n("fresh", 300, 0), n("fixedfresh", 300, 350)}
	fresh := notes.NewSince(all, 200)
	require.Len(t, fresh, 1)
	assert.Equal(t, "fresh", fresh[0].ID, "fixed notes are not actionable even when fresh")
}

func TestScreenshotPathPointsWhereTheHarnessSavedIt(t *testing.T) {
	t.Parallel()
	got := notes.ScreenshotPath("/tmp/eyesore-out", "es_1_ab")
	assert.Equal(t, "/tmp/eyesore-out/screenshots/es_1_ab.png", got)
}

func TestHostileNoteIDsCannotEscapeTheScreenshotsDir(t *testing.T) {
	t.Parallel()
	// dispatch is a network endpoint: ids are attacker-controlled input
	for _, id := range []string{"../../../etc/cron.d/x", "..", "a/b", `a\b`, ""} {
		got := notes.ScreenshotPath("/tmp/eyesore-out", id)
		assert.True(t, strings.HasPrefix(got, "/tmp/eyesore-out/screenshots/"), got)
		assert.NotContains(t, got[len("/tmp/eyesore-out/screenshots/"):], "/", got)
	}
}

func TestParseNotesRoundTripsHarnessDispatchPayloads(t *testing.T) {
	t.Parallel()
	payload := `[{"id":"es_1","selector":"#app","label":"App","note":"test",` +
		`"url":"http://localhost","createdAt":1,"editedAt":1,"dispatchedAt":5}]`
	got, ok := notes.Parse([]byte(payload))
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, "es_1", got[0].ID)
	assert.Equal(t, int64(5), got[0].DispatchedAt)
	assert.Zero(t, got[0].FixedAt, "harness payloads have no fixed status yet")
}

func TestParseRejectsMalformedPayloadsInsteadOfCorruptingTheStore(t *testing.T) {
	t.Parallel()
	_, ok := notes.Parse([]byte("not json"))
	assert.False(t, ok)
}

func TestDuplicateIDsInOneDispatchCollapseToTheLastVersion(t *testing.T) {
	t.Parallel()
	first := n("a", 100, 0)
	second := n("a", 200, 0)
	second.Note = "final text"
	merged := notes.Merge(nil, []notes.Note{first, second})
	require.Len(t, merged, 1)
	assert.Equal(t, "final text", merged[0].Note)
}

func TestFixedStatusSurvivesAStoreRoundTrip(t *testing.T) {
	t.Parallel()
	fixed := n("a", 100, 500)
	data, err := json.Marshal([]notes.Note{fixed})
	require.NoError(t, err)
	got, ok := notes.Parse(data)
	require.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, int64(500), got[0].FixedAt)
}

func TestParseAcceptsAnEmptyDispatch(t *testing.T) {
	t.Parallel()
	got, ok := notes.Parse([]byte("[]"))
	require.True(t, ok)
	assert.Empty(t, got)
}
