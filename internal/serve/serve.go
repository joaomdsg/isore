// Package serve holds the MCP tool handlers, kept free of MCP SDK types so
// the tool logic is testable; cmd/isore adapts them onto the SDK.
package serve

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/joaomdsg/isore/internal/notes"
	"github.com/joaomdsg/isore/internal/store"
)

// NoteView is a pending note as presented to an agent.
type NoteView struct {
	ID           string `json:"id" jsonschema:"note id, pass to mark_fixed"`
	Selector     string `json:"selector" jsonschema:"CSS selector of the annotated element"`
	Label        string `json:"label" jsonschema:"visible text of the element"`
	Note         string `json:"note" jsonschema:"what the user wants changed"`
	URL          string `json:"url" jsonschema:"page the note was made on"`
	DispatchedAt int64  `json:"dispatchedAt"`
	Screenshot   string `json:"screenshot" jsonschema:"path to the element screenshot PNG"`

	Comments []notes.Comment `json:"comments,omitempty" jsonschema:"follow-up remarks the user attached to this note after dispatch, oldest first — read these for extra context instead of expecting the note text itself to change"`

	Summary string `json:"summary,omitempty" jsonschema:"your most recent summary on this already-fixed note, for context"`
	Nudge   string `json:"nudge,omitempty" jsonschema:"the user's edit of your earlier summary on an already-fixed note — re-work the fix to match it, then mark_fixed again with the corrected summary"`

	Thread []notes.ThreadEntry `json:"thread,omitempty" jsonschema:"chronological history of your summaries and the user's nudges on this note, oldest first"`
}

// Handlers implements the MCP tools over a shared note store.
type Handlers struct {
	store  *store.Store
	outDir string
	now    func() int64

	mu        sync.Mutex
	proxyBase string
}

func New(s *store.Store, outDir string, now func() int64) *Handlers {
	return &Handlers{store: s, outDir: outDir, now: now}
}

// SetProxyBase points Await at a running proxy's SSE stream (its
// /__isore/events endpoint) — call it once start_proxy has bound an address.
func (h *Handlers) SetProxyBase(base string) {
	h.mu.Lock()
	h.proxyBase = base
	h.mu.Unlock()
}

// ListNotes returns all pending notes.
func (h *Handlers) ListNotes(_ context.Context) ([]NoteView, error) {
	all, err := h.store.Load()
	if err != nil {
		return nil, err
	}
	return h.views(notes.Pending(all)), nil
}

// MarkFixed stamps a note fixed with the server clock.
func (h *Handlers) MarkFixed(_ context.Context, id, summary string) error {
	return h.store.MarkFixed(id, h.now(), summary)
}

// Follow is the sole listener path: it keeps ONE SSE connection to the proxy
// open and invokes emit once per dispatch batch newer than since, advancing
// the checkpoint past each batch so a dispatch is never delivered twice.
// since==0 checkpoints at the latest dispatch or correction dispatch already
// in the store, so only genuinely new work wakes the agent (the checkpoint
// spans fixed notes too, which is safe because the overlay re-stamps
// DispatchedAt on every dispatch — a reopened note always lands above it).
// It returns the checkpoint it reached together with an error only when ctx is
// done (clean stop, nil error) or the stream breaks — never between dispatches.
// The caller (isore subscribe) runs it as a single long-lived listener and, on
// a clean reconnect, resumes from the returned checkpoint so already-seen
// batches are not replayed. It requires a running proxy (SetProxyBase, set by
// start_proxy); without a reachable one it errors instead of silently polling
// the store file, so the agent knows to call start_proxy first.
func (h *Handlers) Follow(ctx context.Context, since int64, emit func([]NoteView) error) (int64, error) {
	all, err := h.store.Load()
	if err != nil {
		return since, err
	}
	if since == 0 {
		since = maxDispatched(all, since)
	}
	// Drain anything already pending before blocking on the stream, so a
	// dispatch that landed before the listener attached is not missed.
	if fresh := notes.NewSince(all, since); len(fresh) > 0 {
		if err := emit(h.views(fresh)); err != nil {
			return since, err
		}
		since = maxDispatched(fresh, since)
	}

	h.mu.Lock()
	base := h.proxyBase
	h.mu.Unlock()
	if base == "" {
		return since, fmt.Errorf("no proxy running — call start_proxy first")
	}
	since, err = h.streamSSE(ctx, base, since, emit)
	if err != nil {
		return since, fmt.Errorf("subscribe via proxy at %s (call start_proxy first if it isn't running): %w", base, err)
	}
	return since, nil
}

// streamSSE ranges the proxy's /__isore/events stream, emitting every "notes"
// event carrying notes dispatched after the running checkpoint. It does not
// return on the first batch — it keeps reading the same connection until ctx
// ends or the stream errors, so one attach serves an entire session's
// dispatches.
func (h *Handlers) streamSSE(ctx context.Context, base string, since int64, emit func([]NoteView) error) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/__isore/events", nil)
	if err != nil {
		return since, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return since, err
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	var eventName string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			eventName = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if eventName != "notes" {
				continue
			}
			var changed []notes.Note
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &changed); err != nil {
				continue
			}
			fresh := notes.NewSince(changed, since)
			if len(fresh) == 0 {
				continue
			}
			if err := emit(h.views(fresh)); err != nil {
				return since, err
			}
			since = maxDispatched(fresh, since)
		}
	}
	if ctx.Err() != nil {
		return since, nil
	}
	return since, sc.Err()
}

// maxDispatched returns the larger of since and the newest DispatchedAt across
// ns — the checkpoint past which those notes will not re-trigger. The overlay
// re-stamps DispatchedAt on every hand-off (first dispatch or nudge), so this
// one field is the whole watermark.
func maxDispatched(ns []notes.Note, since int64) int64 {
	for _, n := range ns {
		if n.DispatchedAt > since {
			since = n.DispatchedAt
		}
	}
	return since
}

func (h *Handlers) views(ns []notes.Note) []NoteView {
	out := make([]NoteView, 0, len(ns))
	for _, n := range ns {
		out = append(out, NoteView{
			ID: n.ID, Selector: n.Selector, Label: n.Label, Note: n.Note,
			URL: n.URL, DispatchedAt: n.DispatchedAt,
			Screenshot: notes.ScreenshotPath(h.outDir, n.ID),
			Comments:   n.Comments,
			Summary:    notes.LatestSummary(n), Nudge: notes.PendingNudge(n), Thread: n.Thread,
		})
	}
	return out
}
