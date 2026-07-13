package browser_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/browser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Integration: exercises a real headless Chromium; skipped when none is
// installed. This is the wiring verification for the CDP driver.
func TestDriverGivesTheAgentEyesAndHands(t *testing.T) {
	if browser.FindChrome() == "" {
		t.Skip("no chromium/chrome binary installed")
	}
	t.Parallel()

	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><h1 id="title">hello eyesore</h1>` +
			`<script>console.log("booted", 42)</script></body></html>`))
	}))
	t.Cleanup(page.Close)

	d, err := browser.Launch(context.Background(), browser.FindChrome())
	require.NoError(t, err)
	t.Cleanup(d.Close)

	require.NoError(t, d.Navigate(page.URL))

	out, err := d.Eval("1+41")
	require.NoError(t, err)
	assert.Equal(t, "42", string(out))

	html, err := d.HTML("#title")
	require.NoError(t, err)
	assert.Contains(t, html, "hello eyesore")

	png, err := d.Screenshot("#title")
	require.NoError(t, err)
	assert.True(t, len(png) > 100 && string(png[1:4]) == "PNG", "element capture must be a real PNG")

	full, err := d.Screenshot("")
	require.NoError(t, err)
	assert.True(t, len(full) > 100, "viewport capture must work too")

	require.Eventually(t, func() bool {
		for _, line := range d.Console.Last(50) {
			if strings.Contains(line, "booted") && strings.Contains(line, "42") {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "console output must reach the agent")
}
