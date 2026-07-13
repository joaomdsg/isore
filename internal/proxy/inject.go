// Package proxy is the injecting reverse proxy between the user's browser and
// their dev server: it plants the overlay into HTML, accepts note dispatches
// over HTTP, and pushes agent activity and reloads back over SSE.
package proxy

import (
	"bytes"
	"strings"
)

const overlayTag = `<script src="/__eyesore/overlay.js"></script>`

// InjectOverlay plants the overlay script tag before the last </body> of an
// HTML document (appending when none exists). Non-HTML bodies pass through
// untouched. The body must be uncompressed — the proxy strips Accept-Encoding
// upstream to guarantee that.
func InjectOverlay(contentType string, body []byte) ([]byte, bool) {
	if !isHTML(contentType) {
		return body, false
	}
	// ASCII-fold only: Unicode lowering can shrink runes (e.g. U+212A) and
	// shift offsets; "</body>" is pure ASCII so byte positions must not move.
	idx := bytes.LastIndex(asciiLower(body), []byte("</body>"))
	if idx < 0 {
		return append(body[:len(body):len(body)], overlayTag...), true
	}
	out := make([]byte, 0, len(body)+len(overlayTag))
	out = append(out, body[:idx]...)
	out = append(out, overlayTag...)
	out = append(out, body[idx:]...)
	return out, true
}

func isHTML(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(strings.TrimSpace(ct), "text/html")
}

func asciiLower(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return out
}
