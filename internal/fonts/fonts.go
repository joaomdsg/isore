// Package fonts embeds the brand typeface files (WOFF2) so the proxy can
// serve them without a network fetch or a separate asset pipeline.
package fonts

import "embed"

//go:embed *.woff2
var files embed.FS

// Get returns the WOFF2 bytes for name (e.g. "space-grotesk-600.woff2"), or
// ok=false if no such file is embedded.
func Get(name string) (data []byte, ok bool) {
	b, err := files.ReadFile(name)
	if err != nil {
		return nil, false
	}
	return b, true
}
