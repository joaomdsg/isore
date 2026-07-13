package serve

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/joaomdsg/eyesore/internal/notes"
)

// Screenshot returns the element PNG captured for a dispatched note. The id
// must exist in the store — agent input never resolves straight into a path.
func (h *Handlers) Screenshot(_ context.Context, id string) ([]byte, error) {
	all, err := h.store.Load()
	if err != nil {
		return nil, err
	}
	known := false
	for _, n := range all {
		if n.ID == id {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf("no note with id %q", id)
	}
	data, err := os.ReadFile(notes.ScreenshotPath(h.outDir, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no screenshot for note %q: use the selector and browser tools instead", id)
	}
	return data, err
}
