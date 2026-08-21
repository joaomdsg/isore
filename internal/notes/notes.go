// Package notes holds the annotation domain: the Note record dispatched by
// the browser overlay and the pure operations the store and MCP server apply
// to collections of them.
package notes

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Comment is a follow-up the user attaches to an already-dispatched note,
// instead of rewriting the note text. It gives the agent the running
// conversation on a note rather than a single overwritten description.
type Comment struct {
	Text string `json:"text"`
	At   int64  `json:"at"`
}

// Thread entry kinds.
const (
	EntrySummary = "summary" // an agent outcome, appended on mark_fixed
	EntryNudge   = "nudge"   // a user revision of the prior summary
)

// ThreadEntry is one item in a note's chronological history. A summary is an
// agent outcome appended when work is marked fixed; a nudge is the user's edit
// of the prior summary, kept as Base→Text so the overlay can render it as a
// tracked-change diff. The thread never overwrites — every fix and every nudge
// appends — so the whole back-and-forth on a note is preserved and shown.
type ThreadEntry struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
	Base string `json:"base,omitempty"` // nudge only: the summary text it revises
	At   int64  `json:"at"`
}

// Note is one UI annotation. FixedAt is zero until an agent marks it fixed.
type Note struct {
	ID           string    `json:"id"`
	Selector     string    `json:"selector"`
	Label        string    `json:"label"`
	Note         string    `json:"note"`
	URL          string    `json:"url"`
	CreatedAt    int64     `json:"createdAt"`
	EditedAt     int64     `json:"editedAt"`
	DispatchedAt int64     `json:"dispatchedAt"`
	FixedAt      int64     `json:"fixedAt,omitempty"`
	AgentStatus  string    `json:"agentStatus,omitempty"`
	Comments     []Comment `json:"comments,omitempty"`

	// Thread is the note's chronological history of agent summaries and user
	// nudges. A nudge is persisted the moment it is saved — before dispatch —
	// so a page reload cannot lose it; the overlay restores it on reconcile.
	// DispatchedAt is re-stamped when a nudge is handed back to the agent, so
	// a nudge whose At is newer than DispatchedAt is still pending.
	Thread []ThreadEntry `json:"thread,omitempty"`
}

// Open reports whether a note still has outstanding work. A note that has never
// been fixed (FixedAt zero — including one reopened by a re-dispatch, which
// zeroes FixedAt) is open. A fixed note is open again only while its latest
// thread entry is a nudge awaiting a new summary. This single predicate drives
// the overlay's open count, the badge state, and the mark_* guards, so "open"
// means the same thing on both sides of the wire.
func Open(n Note) bool {
	if n.FixedAt == 0 {
		return true
	}
	return len(n.Thread) > 0 && n.Thread[len(n.Thread)-1].Kind != EntrySummary
}

// LatestSummary returns the text of the most recent summary entry, or "".
func LatestSummary(n Note) string {
	for i := len(n.Thread) - 1; i >= 0; i-- {
		if n.Thread[i].Kind == EntrySummary {
			return n.Thread[i].Text
		}
	}
	return ""
}

// PendingNudge returns the text of a nudge awaiting an agent answer (the last
// thread entry is a nudge), or "".
func PendingNudge(n Note) string {
	if len(n.Thread) > 0 {
		if last := n.Thread[len(n.Thread)-1]; last.Kind == EntryNudge {
			return last.Text
		}
	}
	return ""
}

// Merge upserts incoming dispatched notes into the existing store contents.
// An incoming note replaces the stored one wholesale, so re-dispatching a
// fixed note reopens it (incoming FixedAt is zero).
func Merge(existing, incoming []Note) []Note {
	merged := make([]Note, len(existing))
	copy(merged, existing)
	index := map[string]int{}
	for i, e := range merged {
		index[e.ID] = i
	}
	for _, in := range incoming {
		if i, ok := index[in.ID]; ok {
			merged[i] = in
		} else {
			index[in.ID] = len(merged)
			merged = append(merged, in)
		}
	}
	return merged
}

// Pending returns the notes not yet marked fixed.
func Pending(all []Note) []Note {
	var out []Note
	for _, n := range all {
		if n.FixedAt == 0 {
			out = append(out, n)
		}
	}
	return out
}

// MarkFixed appends the agent's summary to the note's thread and stamps it
// fixed at now (unix ms), returning an updated copy. It resolves both a first
// fix and a re-worked nudge: either way the summary becomes the new latest
// entry, closing the note. A fix on an already-settled note (no open work) is
// a no-op, so a duplicate mark_fixed never appends a second summary. An unknown
// id is an error.
func MarkFixed(all []Note, id string, now int64, summary string) ([]Note, error) {
	updated := make([]Note, len(all))
	copy(updated, all)
	for i := range updated {
		if updated[i].ID == id {
			if !Open(updated[i]) {
				return updated, nil
			}
			updated[i].Thread = append(append([]ThreadEntry{}, updated[i].Thread...),
				ThreadEntry{Kind: EntrySummary, Text: summary, At: now})
			updated[i].FixedAt = now
			updated[i].AgentStatus = StatusFixed
			return updated, nil
		}
	}
	return nil, fmt.Errorf("no note with id %q", id)
}

// Delete removes the note with the given id, if present.
func Delete(all []Note, id string) []Note {
	out := make([]Note, 0, len(all))
	for _, n := range all {
		if n.ID != id {
			out = append(out, n)
		}
	}
	return out
}

// AddComment appends a timestamped follow-up to the note with the given id
// and bumps EditedAt, so the note is picked up as changed on the next
// dispatch without its original text being overwritten. An unknown id is an
// error.
func AddComment(all []Note, id string, text string, now int64) ([]Note, error) {
	updated := make([]Note, len(all))
	copy(updated, all)
	for i := range updated {
		if updated[i].ID == id {
			updated[i].Comments = append(append([]Comment{}, updated[i].Comments...), Comment{Text: text, At: now})
			updated[i].EditedAt = now
			return updated, nil
		}
	}
	return nil, fmt.Errorf("no note with id %q", id)
}

// NewSince returns the notes that are new agent work dispatched strictly after
// since (unix ms): open notes whose DispatchedAt is past the checkpoint. This
// spans both a first dispatch (empty thread, FixedAt zero) and a re-dispatched
// nudge, since the overlay re-stamps DispatchedAt on every hand-off. A
// saved-but-undispatched nudge keeps its old DispatchedAt, so it is not work
// yet.
func NewSince(all []Note, since int64) []Note {
	var out []Note
	for _, n := range all {
		if Open(n) && n.DispatchedAt > since {
			out = append(out, n)
		}
	}
	return out
}

// ScreenshotPath is where the harness saves the element screenshot for a note.
// IDs arrive over the network via /__isore/dispatch, so anything outside the
// overlay's [a-zA-Z0-9_-] alphabet is folded away — the result can never
// escape outDir/screenshots.
func ScreenshotPath(outDir, id string) string {
	safe := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-':
			safe = append(safe, c)
		}
	}
	if len(safe) == 0 {
		safe = []byte("_")
	}
	return filepath.Join(outDir, "screenshots", string(safe)+".png")
}

// Parse decodes a dispatch payload or store file into notes.
func Parse(data []byte) ([]Note, bool) {
	var ns []Note
	if err := json.Unmarshal(data, &ns); err != nil {
		return nil, false
	}
	return ns, true
}
