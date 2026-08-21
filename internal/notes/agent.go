package notes

import (
	"fmt"
	"reflect"
)

// Agent activity statuses shown on overlay badges.
const (
	StatusWorking = "working"
	StatusFixed   = "fixed"
)

// MarkWorking flags a pending note as picked up by an agent, returning an
// updated copy. Fixed notes are rejected: they reopen only via re-dispatch.
func MarkWorking(all []Note, id string) ([]Note, error) {
	updated := make([]Note, len(all))
	copy(updated, all)
	for i := range updated {
		if updated[i].ID != id {
			continue
		}
		if !Open(updated[i]) {
			return nil, fmt.Errorf("note %q is already fixed; it reopens only when re-dispatched", id)
		}
		updated[i].AgentStatus = StatusWorking
		return updated, nil
	}
	return nil, fmt.Errorf("no note with id %q", id)
}

// Diff returns the notes in after that are new or changed relative to before,
// i.e. exactly what a connected overlay needs to repaint. Notes deleted
// between before and after are reported separately by Removed.
func Diff(before, after []Note) []Note {
	prev := map[string]Note{}
	for _, b := range before {
		prev[b.ID] = b
	}
	var changed []Note
	for _, a := range after {
		if b, ok := prev[a.ID]; !ok || !reflect.DeepEqual(b, a) {
			changed = append(changed, a)
		}
	}
	return changed
}

// Removed returns the ids present in before but absent from after, i.e. notes
// deleted server-side that a connected overlay must drop from its own state.
func Removed(before, after []Note) []string {
	live := map[string]bool{}
	for _, a := range after {
		live[a.ID] = true
	}
	var gone []string
	for _, b := range before {
		if !live[b.ID] {
			gone = append(gone, b.ID)
		}
	}
	return gone
}
