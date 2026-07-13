package serve_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheAgentCanLookAtTheElementTheUserPointedAt(t *testing.T) {
	t.Parallel()
	s, h, dir := setup(t)
	dispatch(t, s, n("a", 100, 0))
	png := []byte("\x89PNG fake bytes")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "screenshots"), 0o755))
	require.NoError(t, os.WriteFile(notes.ScreenshotPath(dir, "a"), png, 0o644))

	got, err := h.Screenshot(context.Background(), "a")
	require.NoError(t, err)
	assert.Equal(t, png, got)
}

func TestAMissingScreenshotExplainsItselfInsteadOfHandingOverGarbage(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("a", 100, 0))
	_, err := h.Screenshot(context.Background(), "a")
	assert.ErrorContains(t, err, "no screenshot",
		"the agent should fall back to the selector, not crash on absent bytes")
}

func TestAgentSuppliedIDsCannotWalkTheFilesystem(t *testing.T) {
	t.Parallel()
	_, h, _ := setup(t)
	_, err := h.Screenshot(context.Background(), "../../../../etc/hostname")
	assert.Error(t, err, "ids resolve through the store, never straight into a path")
}

func TestScreenshotsAreOnlyServedForRealNotes(t *testing.T) {
	t.Parallel()
	_, h, dir := setup(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "screenshots"), 0o755))
	require.NoError(t, os.WriteFile(notes.ScreenshotPath(dir, "ghost"), []byte("x"), 0o644))
	_, err := h.Screenshot(context.Background(), "ghost")
	assert.Error(t, err, "the id namespace is the store, not the filesystem")
}
