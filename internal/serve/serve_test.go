package serve_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joaomdsg/isore/internal/notes"
	"github.com/joaomdsg/isore/internal/proxy"
	"github.com/joaomdsg/isore/internal/serve"
	"github.com/joaomdsg/isore/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setup wires Handlers to a real proxy (as start_proxy would), since Await
// now requires one instead of falling back to polling the store file.
func setup(t *testing.T) (*store.Store, *serve.Handlers, string) {
	t.Helper()
	dir := t.TempDir()
	s := store.New(filepath.Join(dir, "notes.json"))
	h := serve.New(s, dir, func() int64 { return 7777 })
	target, err := url.Parse("http://127.0.0.1:1")
	require.NoError(t, err)
	// fast internal watch-poll so the proxy's own SSE broadcast fires promptly
	px := proxy.NewServer(target, s, []byte("//overlay"), 20*time.Millisecond)
	ts := httptest.NewServer(px)
	t.Cleanup(ts.Close)
	h.SetProxyBase(ts.URL)
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

// followOnce runs the sole listener (Follow) until it emits the first batch
// (returned), the wait window elapses with nothing dispatched (empty result),
// or Follow errors before emitting (error returned) — the one-shot view used
// to assert the checkpoint/correction semantics the listener must honor.
func followOnce(t *testing.T, h *serve.Handlers, since int64, wait time.Duration) ([]serve.NoteView, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []serve.NoteView, 1)
	errc := make(chan error, 1)
	go func() {
		_, err := h.Follow(ctx, since, func(ns []serve.NoteView) error {
			select {
			case got <- ns:
			default:
			}
			cancel() // stop after the first batch
			return nil
		})
		errc <- err
	}()
	select {
	case b := <-got:
		return b, nil
	case err := <-errc:
		// Follow returned; if it emitted a batch first, prefer that.
		select {
		case b := <-got:
			return b, nil
		default:
			return nil, err
		}
	case <-time.After(wait):
		return nil, nil
	}
}

func TestFollowCheckpointsAtTheLatestKnownDispatchSoOldNotesDoNotRetrigger(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("early", 100, 0), n("late", 400, 0))

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = s.Merge([]notes.Note{n("between", 250, 0)})
	}()

	got, err := followOnce(t, h, 0, 500*time.Millisecond)
	require.NoError(t, err)
	assert.Empty(t, got,
		"checkpoint is the MAX known dispatch time; anything at or below it is old news")
}

func TestFollowDeliversNotesDispatchedWhileTheAgentWaits(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("old", 100, 0))

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = s.Merge([]notes.Note{n("fresh", 999, 0)})
	}()

	got, err := followOnce(t, h, 0, 3*time.Second)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "fresh", got[0].ID)
	assert.NotEmpty(t, got[0].Screenshot)
}

func TestFollowOnAnEmptyStoreCatchesTheVeryFirstDispatch(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = s.Merge([]notes.Note{n("first", 1, 0)})
	}()

	got, err := followOnce(t, h, 0, 3*time.Second)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "first", got[0].ID,
		"an empty store checkpoints at zero, so even a tiny dispatch time counts as fresh")
}

func TestFollowWithExplicitCheckpointReplaysNotesTheAgentMissed(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("missed", 300, 0))

	got, err := followOnce(t, h, 200, 3*time.Second)
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

	_, err = followOnce(t, h, 0, 200*time.Millisecond)
	assert.Error(t, err)

	_, err = followOnce(t, h, 1, 200*time.Millisecond)
	assert.Error(t, err, "corruption while listening must also surface")
}

// TestFollowErrorsWithoutARunningProxy proves there is no silent poll
// fallback: an agent that subscribes before start_proxy must get a
// clear, actionable error instead of a wait that happens to work anyway.
func TestFollowErrorsWithoutARunningProxy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	h := serve.New(store.New(filepath.Join(dir, "notes.json")), dir, func() int64 { return 7777 })

	_, err := followOnce(t, h, 0, 200*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start_proxy")
}

func TestFollowCheckpointsPastDispatchedCorrectionsSoResolvedOnesDoNotRetrigger(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	corrected := n("a", 100, 500)
	corrected.Thread = []notes.ThreadEntry{
		{Kind: notes.EntrySummary, Text: "old summary", At: 500},
		{Kind: notes.EntryNudge, Base: "old summary", Text: "tightened summary", At: 550},
	}
	corrected.DispatchedAt = 600 // nudge re-dispatched, later than every prior DispatchedAt
	dispatch(t, s, corrected)

	got, err := followOnce(t, h, 0, 400*time.Millisecond)
	require.NoError(t, err)
	assert.Empty(t, got,
		"since==0 checkpoint must span re-dispatched nudges too, or every listen re-delivers them")
}

func recvBatch(t *testing.T, ch <-chan []serve.NoteView) []serve.NoteView {
	t.Helper()
	select {
	case b := <-ch:
		return b
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a dispatch batch")
		return nil
	}
}

// Follow is the persistent listener the agent runs once as a background command:
// it must stay on ONE open SSE connection and hand back every dispatch in turn,
// never returning between them. If Follow returned after the first batch, the
// second dispatch would never arrive and recvBatch would time out.
func TestFollowStreamsEveryDispatchOnOneConnection(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("old", 100, 0)) // pre-existing: since=0 must checkpoint past it

	batches := make(chan []serve.NoteView, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := h.Follow(ctx, 0, func(ns []serve.NoteView) error {
			batches <- ns
			return nil
		})
		done <- err
	}()

	time.Sleep(30 * time.Millisecond) // let the listener attach to the stream
	require.NoError(t, s.Merge([]notes.Note{n("first", 200, 0)}))
	b1 := recvBatch(t, batches)
	require.Len(t, b1, 1)
	assert.Equal(t, "first", b1[0].ID)

	require.NoError(t, s.Merge([]notes.Note{n("second", 300, 0)}))
	b2 := recvBatch(t, batches)
	require.Len(t, b2, 1)
	assert.Equal(t, "second", b2[0].ID, "the same connection must keep delivering later dispatches")

	// The pre-existing "old" note must never be delivered.
	select {
	case extra := <-batches:
		t.Fatalf("unexpected extra batch delivered: %+v", extra)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	require.NoError(t, <-done, "ctx cancel is a clean stop, not an error")
}

func TestFollowDeliversADispatchedNudgeWithItsTextAndPriorSummary(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	corrected := n("a", 100, 500)
	corrected.Thread = []notes.ThreadEntry{
		{Kind: notes.EntrySummary, Text: "old summary", At: 300},
		{Kind: notes.EntryNudge, Base: "old summary", Text: "user's edited summary", At: 350},
	}
	corrected.DispatchedAt = 400
	dispatch(t, s, corrected)

	got, err := followOnce(t, h, 200, 3*time.Second)
	require.NoError(t, err)
	require.Len(t, got, 1, "a nudge dispatched after the checkpoint is new agent work")
	assert.Equal(t, "user's edited summary", got[0].Nudge,
		"the agent must see the nudge text to act on it")
	assert.Equal(t, "old summary", got[0].Summary,
		"the agent sees its prior summary for context")
	assert.Len(t, got[0].Thread, 2, "the full thread rides along")
}
