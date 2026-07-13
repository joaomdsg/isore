package serve_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/joaomdsg/eyesore/internal/notes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkWorkingLightsUpTheUsersBadge(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("a", 100, 0))

	require.NoError(t, h.MarkWorking(context.Background(), "a"))

	all, err := s.Load()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, notes.StatusWorking, all[0].AgentStatus)
}

func TestMarkWorkingOnUnknownOrFixedNotesTellsTheAgent(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("done", 100, 500))
	assert.Error(t, h.MarkWorking(context.Background(), "ghost"))
	assert.Error(t, h.MarkWorking(context.Background(), "done"))

	all, err := s.Load()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Empty(t, all[0].AgentStatus, "failed marks must not leave droppings in the store")
}

func TestTheAgentsSummaryReachesTheStoreForTheOverlayToShow(t *testing.T) {
	t.Parallel()
	s, h, _ := setup(t)
	dispatch(t, s, n("a", 100, 0))

	require.NoError(t, h.MarkFixed(context.Background(), "a", "made it green"))

	all, err := s.Load()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "made it green", all[0].AgentSummary)
	assert.Equal(t, notes.StatusFixed, all[0].AgentStatus)
}

func TestReloadReachesTheProxySoTheUsersPageRefreshes(t *testing.T) {
	t.Parallel()
	hits := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.Method + " " + r.URL.Path
	}))
	t.Cleanup(proxy.Close)

	_, h, _ := setup(t)
	h.ReloadURL = proxy.URL + "/__eyesore/reload"
	require.NoError(t, h.Reload(context.Background()))
	assert.Equal(t, "POST /__eyesore/reload", <-hits)
}

func TestReloadWithoutAProxyExplainsItselfInsteadOfPretendingSuccess(t *testing.T) {
	t.Parallel()
	_, h, _ := setup(t)
	err := h.Reload(context.Background())
	assert.ErrorContains(t, err, "no proxy",
		"chromedp mode has no proxy; the agent must learn the refresh went nowhere")
}

func TestReloadHonorsCancellationSoAStuckProxyCannotHangTheAgent(t *testing.T) {
	t.Parallel()
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(blocked.Close)

	_, h, _ := setup(t)
	h.ReloadURL = blocked.URL + "/__eyesore/reload"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, h.Reload(ctx))
}

func TestAnyTwoHundredCountsAsReloaded(t *testing.T) {
	t.Parallel()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)

	_, h, _ := setup(t)
	h.ReloadURL = proxy.URL + "/__eyesore/reload"
	assert.NoError(t, h.Reload(context.Background()))
}

func TestAProxyErrorSurfacesToTheAgent(t *testing.T) {
	t.Parallel()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(proxy.Close)

	_, h, _ := setup(t)
	h.ReloadURL = proxy.URL + "/__eyesore/reload"
	assert.Error(t, h.Reload(context.Background()))
}
