package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/joaomdsg/eyesore/internal/store"
)

const maxDispatchBytes = 4 << 20

type event struct {
	name string
	data string
}

// ShootFunc captures an element screenshot on the given page.
type ShootFunc func(url, selector string) ([]byte, error)

// Option configures a Server.
type Option func(*Server)

// WithShooter enables best-effort element screenshots on dispatch.
func WithShooter(shoot ShootFunc) Option {
	return func(s *Server) { s.shoot = shoot }
}

// Server is the injecting reverse proxy plus the overlay's HTTP/SSE endpoints.
type Server struct {
	mux     *http.ServeMux
	store   *store.Store
	overlay []byte

	shoot ShootFunc

	mu   sync.Mutex
	subs map[chan event]struct{}

	done      chan struct{}
	closeOnce sync.Once
	inflight  sync.WaitGroup
}

// NewServer proxies to target, injecting the overlay into HTML and serving the
// /__eyesore/ control endpoints. A watcher polls the store every poll interval
// and pushes note changes to SSE subscribers; Close stops it.
func NewServer(target *url.URL, st *store.Store, overlay []byte, poll time.Duration, opts ...Option) *Server {
	s := &Server{
		mux:     http.NewServeMux(),
		store:   st,
		overlay: overlay,
		subs:    map[chan event]struct{}{},
		done:    make(chan struct{}),
	}

	rp := httputil.NewSingleHostReverseProxy(target)
	baseDirector := rp.Director
	rp.Director = func(req *http.Request) {
		baseDirector(req)
		// identity only: injection must never splice into compressed bodies
		req.Header.Del("Accept-Encoding")
	}
	// the default transport silently re-adds "Accept-Encoding: gzip"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	rp.Transport = transport
	rp.ModifyResponse = func(resp *http.Response) error {
		// Buffer-and-inject only plain HTML documents. Upgrades (websockets),
		// streams (SSE/HMR), and already-compressed bodies must flow untouched:
		// reading them to EOF would hang, and splicing into them corrupts.
		if resp.StatusCode == http.StatusSwitchingProtocols ||
			resp.Header.Get("Content-Encoding") != "" ||
			!isHTML(resp.Header.Get("Content-Type")) {
			return nil
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		resp.Body.Close()
		out, _ := InjectOverlay(resp.Header.Get("Content-Type"), body)
		resp.Body = io.NopCloser(bytes.NewReader(out))
		resp.ContentLength = int64(len(out))
		resp.Header.Set("Content-Length", strconv.Itoa(len(out)))
		return nil
	}

	s.mux.HandleFunc("GET /__eyesore/overlay.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(s.overlay)
	})
	s.mux.HandleFunc("POST /__eyesore/dispatch", s.handleDispatch)
	s.mux.HandleFunc("POST /__eyesore/reload", func(w http.ResponseWriter, r *http.Request) {
		s.broadcast(event{name: "reload", data: "{}"})
		w.WriteHeader(http.StatusNoContent)
	})
	s.mux.HandleFunc("GET /__eyesore/events", s.handleEvents)
	s.mux.Handle("/", rp)

	for _, opt := range opts {
		opt(s)
	}
	go s.watch(poll)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

// Close stops the store watcher and waits for in-flight screenshot captures.
// Safe to call from concurrent shutdown paths.
func (s *Server) Close() {
	s.closeOnce.Do(func() { close(s.done) })
	s.inflight.Wait()
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxDispatchBytes))
	if err != nil {
		http.Error(w, "read: "+err.Error(), http.StatusBadRequest)
		return
	}
	dispatched, ok := notes.Parse(body)
	if !ok {
		http.Error(w, "body must be a JSON note list", http.StatusBadRequest)
		return
	}
	if err := s.store.Merge(dispatched); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if s.shoot != nil {
		s.inflight.Add(1)
		go func() {
			defer s.inflight.Done()
			s.captureAll(dispatched)
		}()
	}
	w.WriteHeader(http.StatusNoContent)
}

// captureAll saves best-effort element screenshots; the notes already landed,
// so a dead browser only costs the images, never the dispatch.
func (s *Server) captureAll(dispatched []notes.Note) {
	ssDir := filepath.Join(s.store.Dir(), "screenshots")
	if err := os.MkdirAll(ssDir, 0o755); err != nil {
		return
	}
	for _, n := range dispatched {
		select {
		case <-s.done:
			return
		default:
		}
		png, err := s.shoot(n.URL, n.Selector)
		if err != nil || len(png) == 0 {
			continue
		}
		_ = os.WriteFile(notes.ScreenshotPath(s.store.Dir(), n.ID), png, 0o644)
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	sub := make(chan event, 16)
	s.mu.Lock()
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, sub)
		s.mu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case ev := <-sub:
			// LF-only framing per the SSE spec
			if _, err := io.WriteString(w, "event: "+ev.name+"\ndata: "+ev.data+"\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) broadcast(ev event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sub := range s.subs {
		select {
		case sub <- ev:
		default: // a stalled tab must not block the rest
		}
	}
}

// watch polls the store and pushes note diffs to subscribers. Load errors keep
// the previous snapshot: corruption must not spray empty repaints.
func (s *Server) watch(poll time.Duration) {
	snapshot, _ := s.store.Load()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-tick.C:
			current, err := s.store.Load()
			if err != nil {
				continue
			}
			if changed := notes.Diff(snapshot, current); len(changed) > 0 {
				data, err := json.Marshal(changed)
				if err != nil {
					continue
				}
				s.broadcast(event{name: "notes", data: string(data)})
			}
			snapshot = current
		}
	}
}
