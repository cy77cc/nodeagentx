package federation

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanaryStrategy_SelectSubset_PercentageBased(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60, AutoRollback: true},
	})
	agents := make([]string, 100)
	for i := range agents {
		agents[i] = fmt.Sprintf("agent-%03d", i)
	}
	subset, err := cs.SelectSubset(agents, 0)
	require.NoError(t, err)
	assert.Len(t, subset, 10)
}

func TestCanaryStrategy_SelectSubset_RoundsCorrectly(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60},
	})
	agents := make([]string, 15)
	for i := range agents {
		agents[i] = fmt.Sprintf("agent-%03d", i)
	}
	subset, err := cs.SelectSubset(agents, 0)
	require.NoError(t, err)
	assert.Len(t, subset, 2)
}

func TestCanaryStrategy_SelectSubset_SecondStage(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60},
		{Percentage: 30, WaitSeconds: 120},
	})
	agents := make([]string, 100)
	for i := range agents {
		agents[i] = fmt.Sprintf("agent-%03d", i)
	}
	subset, err := cs.SelectSubset(agents, 1)
	require.NoError(t, err)
	assert.Len(t, subset, 30)
}

func TestCanaryStrategy_SelectSubset_OutOfRange(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60},
	})
	_, err := cs.SelectSubset([]string{"a", "b"}, 1)
	assert.Error(t, err)
}

func TestCanaryStrategy_TotalStages(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10},
		{Percentage: 30},
		{Percentage: 100},
	})
	assert.Equal(t, 3, cs.TotalStages())
}
