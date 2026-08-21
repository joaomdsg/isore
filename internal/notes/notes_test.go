package notes_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/joaomdsg/isore/internal/notes"
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
	got := notes.ScreenshotPath("/tmp/isore-out", "es_1_ab")
	assert.Equal(t, "/tmp/isore-out/screenshots/es_1_ab.png", got)
}

func TestHostileNoteIDsCannotEscapeTheScreenshotsDir(t *testing.T) {
	t.Parallel()
	// dispatch is a network endpoint: ids are attacker-controlled input
	for _, id := range []string{"../../../etc/cron.d/x", "..", "a/b", `a\b`, ""} {
		got := notes.ScreenshotPath("/tmp/isore-out", id)
		assert.True(t, strings.HasPrefix(got, "/tmp/isore-out/screenshots/"), got)
		assert.NotContains(t, got[len("/tmp/isore-out/screenshots/"):], "/", got)
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

func TestDeleteRemovesOnlyTheTargetNote(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0), n("b", 100, 0)}
	got := notes.Delete(all, "a")
	require.Len(t, got, 1)
	assert.Equal(t, "b", got[0].ID)
}

func TestDeleteOfUnknownIDIsANoOp(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0)}
	got := notes.Delete(all, "nope")
	assert.Len(t, got, 1)
}

func TestAddCommentAppendsWithoutOverwritingNoteText(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0)}
	updated, err := notes.AddComment(all, "a", "also check the footer", 500)
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "note a", updated[0].Note, "comment must not overwrite the original note text")
	require.Len(t, updated[0].Comments, 1)
	assert.Equal(t, "also check the footer", updated[0].Comments[0].Text)
	assert.Equal(t, int64(500), updated[0].Comments[0].At)
	assert.Equal(t, int64(500), updated[0].EditedAt, "comment bumps editedAt so it is re-dispatched")
	assert.Empty(t, all[0].Comments, "caller's slice must not be mutated")
}

func TestAddCommentAccumulatesAcrossCalls(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0)}
	once, err := notes.AddComment(all, "a", "first", 500)
	require.NoError(t, err)
	twice, err := notes.AddComment(once, "a", "second", 600)
	require.NoError(t, err)
	require.Len(t, twice[0].Comments, 2)
	assert.Equal(t, "first", twice[0].Comments[0].Text)
	assert.Equal(t, "second", twice[0].Comments[1].Text)
}

func TestAddCommentOnUnknownIDFails(t *testing.T) {
	t.Parallel()
	_, err := notes.AddComment([]notes.Note{n("a", 100, 0)}, "nope", "x", 500)
	assert.Error(t, err)
}

func withThread(base notes.Note, entries ...notes.ThreadEntry) notes.Note {
	base.Thread = append([]notes.ThreadEntry{}, entries...)
	return base
}

func TestPendingNudgesArePersistedSoAReloadCannotLoseThem(t *testing.T) {
	t.Parallel()
	// A saved-but-undispatched nudge is POSTed and stored server-side; the
	// overlay restores it from /__isore/notes on reconcile after a reload.
	fixed := withThread(n("a", 100, 150),
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "Snapped to the baseline.", At: 150})
	nudged := withThread(fixed,
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "Snapped to the baseline.", At: 150},
		notes.ThreadEntry{Kind: notes.EntryNudge, Base: "Snapped to the baseline.", Text: "Snapped to the baseline, aligned to the 8px grid.", At: 300})

	merged := notes.Merge([]notes.Note{fixed}, []notes.Note{nudged})

	require.Len(t, merged, 1)
	assert.Equal(t, "Snapped to the baseline, aligned to the 8px grid.", notes.PendingNudge(merged[0]))
	assert.EqualValues(t, 100, merged[0].DispatchedAt, "an undispatched nudge keeps the old dispatch stamp")
	assert.True(t, notes.Open(merged[0]), "a pending nudge reopens the note")

	round, ok := notes.Parse(mustJSON(t, merged))
	require.True(t, ok)
	assert.Equal(t, notes.PendingNudge(merged[0]), notes.PendingNudge(round[0]), "the nudge must survive a JSON round-trip")
}

func TestNewSinceDeliversDispatchedNudgesButNotPendingOnes(t *testing.T) {
	t.Parallel()
	dispatched := withThread(n("a", 100, 150),
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "s", At: 150},
		notes.ThreadEntry{Kind: notes.EntryNudge, Base: "s", Text: "better summary", At: 380})
	dispatched.DispatchedAt = 400 // re-dispatched past the checkpoint
	pending := withThread(n("b", 100, 150),
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "s", At: 150},
		notes.ThreadEntry{Kind: notes.EntryNudge, Base: "s", Text: "draft nudge", At: 350})
	// pending.DispatchedAt stays 100: saved, not yet dispatched.
	plainFixed := withThread(n("c", 100, 150),
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "s", At: 150})

	out := notes.NewSince([]notes.Note{dispatched, pending, plainFixed}, 200)

	require.Len(t, out, 1)
	assert.Equal(t, "a", out[0].ID)
}

func TestMarkFixedResolvesADispatchedNudgeByAppendingANewSummary(t *testing.T) {
	t.Parallel()
	nt := withThread(n("a", 100, 150),
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "old summary", At: 150},
		notes.ThreadEntry{Kind: notes.EntryNudge, Base: "old summary", Text: "new summary", At: 380})
	nt.DispatchedAt = 400
	nt.AgentStatus = notes.StatusWorking

	updated, err := notes.MarkFixed([]notes.Note{nt}, "a", 500, "new summary")

	require.NoError(t, err)
	got := updated[0]
	assert.Equal(t, "new summary", notes.LatestSummary(got))
	assert.Empty(t, notes.PendingNudge(got), "the nudge is answered by the new summary")
	assert.Len(t, got.Thread, 3, "summary, nudge, summary — history is kept, not overwritten")
	assert.EqualValues(t, 500, got.FixedAt, "re-fix restamps so Diff broadcasts the resolution")
	assert.Equal(t, notes.StatusFixed, got.AgentStatus)
	assert.False(t, notes.Open(got))
}

func TestMarkFixedWithoutANudgeIsStillANoOpOnSecondFix(t *testing.T) {
	t.Parallel()
	nt := withThread(n("a", 100, 150),
		notes.ThreadEntry{Kind: notes.EntrySummary, Text: "first summary", At: 150})

	updated, err := notes.MarkFixed([]notes.Note{nt}, "a", 500, "second summary")

	require.NoError(t, err)
	assert.Equal(t, "first summary", notes.LatestSummary(updated[0]))
	assert.Len(t, updated[0].Thread, 1, "no intervening nudge → no second summary")
	assert.EqualValues(t, 150, updated[0].FixedAt)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
