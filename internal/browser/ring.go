// Package browser gives the MCP agent the actual browser: screenshots, JS
// evaluation, DOM reads, navigation, and console output — attached to the
// user's visible Chromium (harness mode) or an owned headless one (proxy mode).
package browser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
)

// Ring keeps the last N console messages, oldest first.
type Ring struct {
	mu  sync.Mutex
	buf []string
	max int
}

func NewRing(max int) *Ring {
	return &Ring{max: max}
}

func (r *Ring) Add(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, msg)
	if len(r.buf) > r.max {
		r.buf = r.buf[len(r.buf)-r.max:]
	}
}

// Last returns up to n newest messages, oldest first.
func (r *Ring) Last(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.buf) {
		n = len(r.buf)
	}
	out := make([]string, n)
	copy(out, r.buf[len(r.buf)-n:])
	return out
}

type endpoint struct {
	DebugURL string `json:"debugURL"`
}

// WriteEndpoint records the browser's CDP debug URL for the MCP process.
func WriteEndpoint(path, debugURL string) error {
	data, err := json.Marshal(endpoint{DebugURL: debugURL})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadEndpoint loads the CDP debug URL written by the harness.
func ReadEndpoint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("no browser attached: start `eyesore` (harness) or use proxy mode, which launches its own")
	}
	if err != nil {
		return "", err
	}
	var e endpoint
	if err := json.Unmarshal(data, &e); err != nil || e.DebugURL == "" {
		return "", fmt.Errorf("no browser endpoint in %s", path)
	}
	return e.DebugURL, nil
}
