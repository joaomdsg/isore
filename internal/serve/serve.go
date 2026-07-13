// Package serve holds the MCP tool handlers, kept free of MCP SDK types so
// the tool logic is testable; cmd/eyesore adapts them onto the SDK.
package serve

import (
	"context"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/store"
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
}

// Handlers implements the MCP tools over a shared note store.
type Handlers struct {
	store  *store.Store
	outDir string
	now    func() int64
	poll   time.Duration
	// ReloadURL is the proxy's reload endpoint; empty outside proxy mode.
	ReloadURL string
}

func New(s *store.Store, outDir string, now func() int64) *Handlers {
	return &Handlers{store: s, outDir: outDir, now: now, poll: 300 * time.Millisecond}
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

// Await blocks until a note dispatched after since (unix ms) appears or ctx
// expires (empty result). since==0 checkpoints at the latest dispatch already
// in the store, so only genuinely new notes wake the agent. The checkpoint
// spans fixed notes too, which is only safe because the overlay re-stamps
// DispatchedAt on every dispatch — a reopened note always lands above it.
func (h *Handlers) Await(ctx context.Context, since int64) ([]NoteView, error) {
	if since == 0 {
		all, err := h.store.Load()
		if err != nil {
			return nil, err
		}
		for _, n := range all {
			if n.DispatchedAt > since {
				since = n.DispatchedAt
			}
		}
	}
	fresh, err := h.store.Await(ctx, since, h.poll)
	if err != nil {
		return nil, err
	}
	return h.views(fresh), nil
}

func (h *Handlers) views(ns []notes.Note) []NoteView {
	out := make([]NoteView, 0, len(ns))
	for _, n := range ns {
		out = append(out, NoteView{
			ID: n.ID, Selector: n.Selector, Label: n.Label, Note: n.Note,
			URL: n.URL, DispatchedAt: n.DispatchedAt,
			Screenshot: notes.ScreenshotPath(h.outDir, n.ID),
		})
	}
	return out
}
