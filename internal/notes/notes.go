// Package notes holds the annotation domain: the Note record dispatched by
// the browser overlay and the pure operations the store and MCP server apply
// to collections of them.
package notes

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// Note is one UI annotation. FixedAt is zero until an agent marks it fixed.
type Note struct {
	ID           string `json:"id"`
	Selector     string `json:"selector"`
	Label        string `json:"label"`
	Note         string `json:"note"`
	URL          string `json:"url"`
	CreatedAt    int64  `json:"createdAt"`
	EditedAt     int64  `json:"editedAt"`
	DispatchedAt int64  `json:"dispatchedAt"`
	FixedAt      int64  `json:"fixedAt,omitempty"`
	AgentStatus  string `json:"agentStatus,omitempty"`
	AgentSummary string `json:"agentSummary,omitempty"`
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

// MarkFixed stamps the note with the given id as fixed at now (unix ms) with
// the agent's summary, returning an updated copy. A second fix is a no-op:
// the first stamp and summary stand. An unknown id is an error.
func MarkFixed(all []Note, id string, now int64, summary string) ([]Note, error) {
	updated := make([]Note, len(all))
	copy(updated, all)
	for i := range updated {
		if updated[i].ID == id {
			if updated[i].FixedAt == 0 {
				updated[i].FixedAt = now
				updated[i].AgentSummary = summary
			}
			updated[i].AgentStatus = StatusFixed
			return updated, nil
		}
	}
	return nil, fmt.Errorf("no note with id %q", id)
}

// NewSince returns pending notes dispatched strictly after since (unix ms).
func NewSince(all []Note, since int64) []Note {
	var out []Note
	for _, n := range all {
		if n.FixedAt == 0 && n.DispatchedAt > since {
			out = append(out, n)
		}
	}
	return out
}

// ScreenshotPath is where the harness saves the element screenshot for a note.
// IDs arrive over the network via /__eyesore/dispatch, so anything outside the
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
