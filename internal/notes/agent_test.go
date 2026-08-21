package notes_test

import (
	"testing"

	"github.com/joaomdsg/isore/internal/notes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkWorkingShowsTheUserTheirNoteWasPickedUp(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0), n("b", 100, 0)}
	updated, err := notes.MarkWorking(all, "a")
	require.NoError(t, err)
	byID := map[string]notes.Note{}
	for _, u := range updated {
		byID[u.ID] = u
	}
	assert.Equal(t, notes.StatusWorking, byID["a"].AgentStatus)
	assert.Empty(t, byID["b"].AgentStatus)
	assert.Empty(t, all[0].AgentStatus, "caller's slice must not be mutated")
}

func TestMarkWorkingOnUnknownNoteFailsLoudly(t *testing.T) {
	t.Parallel()
	_, err := notes.MarkWorking([]notes.Note{n("a", 100, 0)}, "ghost")
	assert.Error(t, err)
}

func TestFixingANoteRecordsTheAgentsSummaryForTheUser(t *testing.T) {
	t.Parallel()
	all := []notes.Note{n("a", 100, 0)}
	working, err := notes.MarkWorking(all, "a")
	require.NoError(t, err)
	fixed, err := notes.MarkFixed(working, "a", 500, "swapped red for brand green")
	require.NoError(t, err)
	require.Len(t, fixed, 1)
	assert.Equal(t, int64(500), fixed[0].FixedAt)
	assert.Equal(t, "swapped red for brand green", notes.LatestSummary(fixed[0]))
	assert.Equal(t, notes.StatusFixed, fixed[0].AgentStatus,
		"the overlay reads one status field to color the badge")
}

func TestASummarylessFixStillColorsTheBadgeFixed(t *testing.T) {
	t.Parallel()
	fixed, err := notes.MarkFixed([]notes.Note{n("a", 100, 0)}, "a", 500, "")
	require.NoError(t, err)
	require.Len(t, fixed, 1)
	assert.Equal(t, notes.StatusFixed, fixed[0].AgentStatus)
}

func TestRefixingASettledNoteIsANoOpSoNoDuplicateSummaryIsAppended(t *testing.T) {
	t.Parallel()
	once, err := notes.MarkFixed([]notes.Note{n("a", 100, 0)}, "a", 500, "first story")
	require.NoError(t, err)
	twice, err := notes.MarkFixed(once, "a", 900, "revised story")
	require.NoError(t, err)
	require.Len(t, twice, 1)
	assert.Len(t, twice[0].Thread, 1, "a fix with no intervening nudge must not append again")
	assert.Equal(t, "first story", notes.LatestSummary(twice[0]))
	assert.Equal(t, int64(500), twice[0].FixedAt, "the original fix time stands")
}

func TestWorkingOnAFixedNoteIsRejectedSoAgentsRedispatchInstead(t *testing.T) {
	t.Parallel()
	fixed, err := notes.MarkFixed([]notes.Note{n("a", 100, 0)}, "a", 500, "done")
	require.NoError(t, err)
	_, err = notes.MarkWorking(fixed, "a")
	assert.Error(t, err, "a fixed note reopens only when the user re-dispatches it")
}

func TestRedispatchingClearsStaleAgentActivity(t *testing.T) {
	t.Parallel()
	existing := []notes.Note{n("a", 100, 0)}
	working, err := notes.MarkWorking(existing, "a")
	require.NoError(t, err)
	merged := notes.Merge(working, []notes.Note{n("a", 200, 0)})
	require.Len(t, merged, 1)
	assert.Empty(t, merged[0].AgentStatus,
		"the user rewrote the note; old agent activity no longer applies")
}

func TestDiffSurfacesExactlyWhatTheOverlayMustRepaint(t *testing.T) {
	t.Parallel()
	before := []notes.Note{n("same", 100, 0), n("progresses", 100, 0), n("gone", 100, 0)}
	after := []notes.Note{n("same", 100, 0), n("progresses", 100, 0), n("new", 300, 0)}
	working, err := notes.MarkWorking(after, "progresses")
	require.NoError(t, err)

	changed := notes.Diff(before, working)

	ids := map[string]bool{}
	for _, c := range changed {
		ids[c.ID] = true
	}
	assert.Equal(t, map[string]bool{"progresses": true, "new": true}, ids,
		"unchanged notes must not repaint; removed notes are the overlay's own doing")
}

func TestDiffOfIdenticalSnapshotsIsQuietSoSSEStaysSilent(t *testing.T) {
	t.Parallel()
	assert.Empty(t, notes.Diff([]notes.Note{n("a", 100, 0)}, []notes.Note{n("a", 100, 0)}))
}

func TestTheFirstSnapshotPaintsEverything(t *testing.T) {
	t.Parallel()
	after := []notes.Note{n("a", 100, 0), n("b", 100, 0)}
	assert.Len(t, notes.Diff(nil, after), 2)
}

func TestAFixArrivingWithNoOtherChangeStillRepaints(t *testing.T) {
	t.Parallel()
	before := []notes.Note{n("a", 100, 0)}
	fixed, err := notes.MarkFixed(before, "a", 500, "done")
	require.NoError(t, err)
	changed := notes.Diff(before, fixed)
	require.Len(t, changed, 1)
	assert.Equal(t, "done", notes.LatestSummary(changed[0]))
}

func TestOpenTracksTheThread(t *testing.T) {
	t.Parallel()
	fresh := n("a", 100, 0)
	assert.True(t, notes.Open(fresh), "a never-fixed note is open")

	fixed, err := notes.MarkFixed([]notes.Note{fresh}, "a", 500, "first pass")
	require.NoError(t, err)
	assert.False(t, notes.Open(fixed[0]), "a fixed note whose last entry is a summary is settled")

	nudged := fixed[0]
	nudged.Thread = append(nudged.Thread, notes.ThreadEntry{Kind: notes.EntryNudge, Base: "first pass", Text: "make it tighter", At: 600})
	assert.True(t, notes.Open(nudged), "a nudge awaiting a new summary reopens the note")
}

func TestNudgeReopensForWorkThenFixAppendsANewSummary(t *testing.T) {
	t.Parallel()
	fixed, err := notes.MarkFixed([]notes.Note{n("a", 100, 0)}, "a", 500, "first pass")
	require.NoError(t, err)
	// The overlay appends the nudge and re-stamps DispatchedAt on dispatch.
	nudged := fixed[0]
	nudged.Thread = append(nudged.Thread, notes.ThreadEntry{Kind: notes.EntryNudge, Base: "first pass", Text: "make it tighter", At: 600})
	nudged.DispatchedAt = 700
	all := []notes.Note{nudged}

	working, err := notes.MarkWorking(all, "a")
	require.NoError(t, err, "a dispatched nudge is open again, so the agent can pick it up")
	assert.Equal(t, notes.StatusWorking, working[0].AgentStatus)

	refixed, err := notes.MarkFixed(working, "a", 900, "second pass")
	require.NoError(t, err)
	assert.Len(t, refixed[0].Thread, 3, "summary, nudge, summary — the whole thread is kept")
	assert.Equal(t, "second pass", notes.LatestSummary(refixed[0]))
	assert.Equal(t, int64(900), refixed[0].FixedAt)
	assert.False(t, notes.Open(refixed[0]))
}

func TestDiffCatchesACommentEvenThoughNoteTextIsUnchanged(t *testing.T) {
	t.Parallel()
	before := []notes.Note{n("a", 100, 0)}
	after, err := notes.AddComment(before, "a", "one more thing", 500)
	require.NoError(t, err)
	changed := notes.Diff(before, after)
	require.Len(t, changed, 1, "a slice-valued field must still be compared for equality")
	assert.Equal(t, "one more thing", changed[0].Comments[0].Text)
}

func TestRemovedReportsDeletedNoteIDs(t *testing.T) {
	t.Parallel()
	before := []notes.Note{n("a", 100, 0), n("b", 100, 0)}
	after := notes.Delete(before, "a")
	assert.Equal(t, []string{"a"}, notes.Removed(before, after))
}

func TestRemovedIsEmptyWhenNothingWasDeleted(t *testing.T) {
	t.Parallel()
	before := []notes.Note{n("a", 100, 0)}
	assert.Empty(t, notes.Removed(before, before))
}
