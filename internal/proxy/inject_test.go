package proxy_test

import (
	"testing"

	"github.com/joaomdsg/eyesore/internal/proxy"
	"github.com/stretchr/testify/assert"
)

const tag = `<script src="/__eyesore/overlay.js"></script>`

func TestOverlayLandsInsideTheDocumentSoItRunsOnEveryPage(t *testing.T) {
	t.Parallel()
	body := `<html><head></head><body><h1>app</h1></body></html>`
	got, injected := proxy.InjectOverlay("text/html; charset=utf-8", []byte(body))
	assert.True(t, injected)
	assert.Equal(t, `<html><head></head><body><h1>app</h1>`+tag+`</body></html>`, string(got))
}

func TestInjectionSurvivesUppercaseMarkupFromLegacyTemplates(t *testing.T) {
	t.Parallel()
	body := `<HTML><BODY>x</BODY></HTML>`
	got, injected := proxy.InjectOverlay("text/html", []byte(body))
	assert.True(t, injected)
	assert.Contains(t, string(got), tag+`</BODY>`)
}

func TestFragmentsWithoutABodyTagStillGetTheOverlay(t *testing.T) {
	t.Parallel()
	body := `<div>bare fragment</div>`
	got, injected := proxy.InjectOverlay("text/html", []byte(body))
	assert.True(t, injected)
	assert.Equal(t, body+tag, string(got))
}

func TestNonHTMLResponsesPassThroughUntouchedSoAssetsDoNotCorrupt(t *testing.T) {
	t.Parallel()
	for _, ct := range []string{"application/json", "text/javascript", "image/png", ""} {
		payload := []byte(`{"a":"</body>"}`)
		got, injected := proxy.InjectOverlay(ct, payload)
		assert.False(t, injected, ct)
		assert.Equal(t, payload, got, ct)
	}
}

func TestContentTypeMatchingIsCaseInsensitiveLikeHTTPItself(t *testing.T) {
	t.Parallel()
	got, injected := proxy.InjectOverlay("Text/HTML;charset=UTF-8", []byte(`<body></body>`))
	assert.True(t, injected)
	assert.Equal(t, `<body>`+tag+`</body>`, string(got))
}

func TestXHTMLIsLeftAloneBecauseScriptInjectionCouldBreakStrictParsers(t *testing.T) {
	t.Parallel()
	payload := []byte(`<html><body/></html>`)
	got, injected := proxy.InjectOverlay("application/xhtml+xml", payload)
	assert.False(t, injected)
	assert.Equal(t, payload, got)
}

func TestAnEmptyHTMLResponseStillCarriesTheOverlay(t *testing.T) {
	t.Parallel()
	got, injected := proxy.InjectOverlay("text/html", nil)
	assert.True(t, injected)
	assert.Equal(t, tag, string(got))
}

func TestMultiByteTextNeverGetsSplicedMidRune(t *testing.T) {
	t.Parallel()
	// U+212A (KELVIN SIGN) shrinks under Unicode lowering; a lowered-copy
	// offset would land the tag inside the rune and corrupt the document.
	body := "<body>KK</body>"
	got, injected := proxy.InjectOverlay("text/html", []byte(body))
	assert.True(t, injected)
	assert.Equal(t, "<body>KK"+tag+"</body>", string(got))
}

func TestOnlyTheLastBodyTagGetsTheOverlaySoNestedMarkupIsSafe(t *testing.T) {
	t.Parallel()
	body := `<body><textarea></body></textarea></body>`
	got, injected := proxy.InjectOverlay("text/html", []byte(body))
	assert.True(t, injected)
	assert.Equal(t, `<body><textarea></body></textarea>`+tag+`</body>`, string(got))
}
