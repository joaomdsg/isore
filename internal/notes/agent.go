package notes

import "fmt"

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
		if updated[i].FixedAt != 0 {
			return nil, fmt.Errorf("note %q is already fixed; it reopens only when re-dispatched", id)
		}
		updated[i].AgentStatus = StatusWorking
		return updated, nil
	}
	return nil, fmt.Errorf("no note with id %q", id)
}

// Diff returns the notes in after that are new or changed relative to before,
// i.e. exactly what a connected overlay needs to repaint. Notes that vanished
// from after are ignored — deletions originate in the overlay itself.
func Diff(before, after []Note) []Note {
	prev := map[string]Note{}
	for _, b := range before {
		prev[b.ID] = b
	}
	var changed []Note
	for _, a := range after {
		if b, ok := prev[a.ID]; !ok || b != a {
			changed = append(changed, a)
		}
	}
	return changed
}
