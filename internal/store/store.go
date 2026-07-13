// Package store persists notes to a JSON file shared between the harness
// process (writes dispatches) and the MCP server process (reads, marks fixed).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
)

// Store is a file-backed note store. Every operation re-reads the file, so
// separate processes pointing at the same path stay consistent.
type Store struct {
	path string
}

func New(path string) *Store {
	return &Store{path: path}
}

// Dir is the directory holding the store file (and its screenshots/ folder).
func (s *Store) Dir() string {
	return filepath.Dir(s.path)
}

// Load reads all notes. A missing file is an empty store.
func (s *Store) Load() ([]notes.Note, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	ns, ok := notes.Parse(data)
	if !ok {
		return nil, fmt.Errorf("corrupt notes store %s: not a JSON note list", s.path)
	}
	return ns, nil
}

// Merge upserts dispatched notes into the store.
func (s *Store) Merge(incoming []notes.Note) error {
	existing, err := s.Load()
	if err != nil {
		return err
	}
	return s.write(notes.Merge(existing, incoming))
}

// MarkFixed stamps a note as fixed at now (unix ms) with the agent's summary
// and persists.
func (s *Store) MarkFixed(id string, now int64, summary string) error {
	all, err := s.Load()
	if err != nil {
		return err
	}
	updated, err := notes.MarkFixed(all, id, now, summary)
	if err != nil {
		return err
	}
	return s.write(updated)
}

// MarkWorking flags a note as picked up by an agent and persists.
func (s *Store) MarkWorking(id string) error {
	all, err := s.Load()
	if err != nil {
		return err
	}
	updated, err := notes.MarkWorking(all, id)
	if err != nil {
		return err
	}
	return s.write(updated)
}

// Await polls until a pending note dispatched after since (unix ms) appears,
// returning the fresh notes. Context expiry is a normal empty result.
func (s *Store) Await(ctx context.Context, since int64, poll time.Duration) ([]notes.Note, error) {
	for {
		all, err := s.Load()
		if err != nil {
			return nil, err
		}
		if fresh := notes.NewSince(all, since); len(fresh) > 0 {
			return fresh, nil
		}
		select {
		case <-ctx.Done():
			return nil, nil
		case <-time.After(poll):
		}
	}
}

// write persists atomically via temp file + rename so a concurrent reader
// never sees a partial JSON document.
func (s *Store) write(all []notes.Note) error {
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".notes-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
