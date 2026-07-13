package browser_test

import (
	"path/filepath"
	"testing"

	"github.com/joaomdsg/eyesore/internal/browser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheAgentSeesRecentConsoleOutputNewestLast(t *testing.T) {
	t.Parallel()
	r := browser.NewRing(3)
	for _, m := range []string{"boot", "warn: a", "error: b", "log: c"} {
		r.Add(m)
	}
	assert.Equal(t, []string{"warn: a", "error: b", "log: c"}, r.Last(10),
		"oldest entries fall off; order of events is preserved")
}

func TestAskingForLessThanEverythingTrimsFromTheOldEnd(t *testing.T) {
	t.Parallel()
	r := browser.NewRing(10)
	r.Add("one")
	r.Add("two")
	r.Add("three")
	assert.Equal(t, []string{"two", "three"}, r.Last(2))
}

func TestAQuietConsoleIsEmptyNotNilPanics(t *testing.T) {
	t.Parallel()
	r := browser.NewRing(4)
	assert.Empty(t, r.Last(5))
	assert.NotPanics(t, func() { r.Last(0) })
}

func TestConcurrentTabsCanLogWhileTheAgentReads(t *testing.T) {
	t.Parallel()
	r := browser.NewRing(64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 500; i++ {
			r.Add("msg")
		}
	}()
	for i := 0; i < 100; i++ {
		r.Last(10)
	}
	<-done
	assert.Len(t, r.Last(1000), 64)
}

func TestTheHandshakeFileConnectsTheTwoProcesses(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "browser.json")
	require.NoError(t, browser.WriteEndpoint(path, "http://127.0.0.1:9222"))
	got, err := browser.ReadEndpoint(path)
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:9222", got)
}

func TestNoHandshakeFileMeansNoAttachedBrowserNotACrash(t *testing.T) {
	t.Parallel()
	_, err := browser.ReadEndpoint(filepath.Join(t.TempDir(), "browser.json"))
	assert.ErrorContains(t, err, "no browser")
}
