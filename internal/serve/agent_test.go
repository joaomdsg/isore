package serve_test

import (
	"context"
	"testing"

	"github.com/joaomdsg/isore/internal/notes"
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
	assert.Equal(t, "made it green", notes.LatestSummary(all[0]))
	assert.Equal(t, notes.StatusFixed, all[0].AgentStatus)
}
