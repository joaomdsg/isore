// Package store persists notes to a JSON file shared between the harness
// process (writes dispatches) and the MCP server process (reads, marks fixed).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/joaomdsg/isore/internal/notes"
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
	return s.locked(func() error {
		existing, err := s.Load()
		if err != nil {
			return err
		}
		return s.write(notes.Merge(existing, incoming))
	})
}

// MarkFixed stamps a note as fixed at now (unix ms) with the agent's summary
// and persists.
func (s *Store) MarkFixed(id string, now int64, summary string) error {
	return s.locked(func() error {
		all, err := s.Load()
		if err != nil {
			return err
		}
		updated, err := notes.MarkFixed(all, id, now, summary)
		if err != nil {
			return err
		}
		return s.write(updated)
	})
}

// MarkWorking flags a note as picked up by an agent and persists.
func (s *Store) MarkWorking(id string) error {
	return s.locked(func() error {
		all, err := s.Load()
		if err != nil {
			return err
		}
		updated, err := notes.MarkWorking(all, id)
		if err != nil {
			return err
		}
		return s.write(updated)
	})
}

// Delete removes a note from the store permanently.
func (s *Store) Delete(id string) error {
	return s.locked(func() error {
		all, err := s.Load()
		if err != nil {
			return err
		}
		return s.write(notes.Delete(all, id))
	})
}

// AddComment appends a timestamped follow-up to a note and persists, without
// touching the note's original text.
func (s *Store) AddComment(id string, text string, now int64) error {
	return s.locked(func() error {
		all, err := s.Load()
		if err != nil {
			return err
		}
		updated, err := notes.AddComment(all, id, text, now)
		if err != nil {
			return err
		}
		return s.write(updated)
	})
}

// locked serializes read-modify-write cycles across goroutines AND processes
// (harness, proxy, MCP server all mutate the same file) via an advisory flock
// on a sidecar; readers stay lock-free thanks to atomic renames.
func (s *Store) locked(fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
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
