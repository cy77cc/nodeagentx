package federation

import (
	"fmt"
	"math"
)

// CanaryStage defines a single stage in a canary rollout.
type CanaryStage struct {
	Percentage   int  `json:"percentage"`
	WaitSeconds  int  `json:"wait_seconds"`
	AutoRollback bool `json:"auto_rollback"`
}

// CanaryStrategy manages staged rollouts to a subset of Leaf agents.
type CanaryStrategy struct {
	stages []CanaryStage
}

// NewCanaryStrategy creates a new CanaryStrategy with the given stages.
func NewCanaryStrategy(stages []CanaryStage) *CanaryStrategy {
	return &CanaryStrategy{stages: stages}
}

// TotalStages returns the number of stages in the strategy.
func (cs *CanaryStrategy) TotalStages() int {
	return len(cs.stages)
}

// GetStage returns the stage at the given index.
func (cs *CanaryStrategy) GetStage(index int) (CanaryStage, error) {
	if index < 0 || index >= len(cs.stages) {
		return CanaryStage{}, fmt.Errorf("stage index %d out of range [0, %d)", index, len(cs.stages))
	}
	return cs.stages[index], nil
}

// SelectSubset returns the cumulative subset of agents for the given stage.
// For stage 0, it returns the first N% of agents.
// For stage N > 0, it returns the first N% of agents (cumulative).
func (cs *CanaryStrategy) SelectSubset(agents []string, stageIndex int) ([]string, error) {
	if stageIndex < 0 || stageIndex >= len(cs.stages) {
		return nil, fmt.Errorf("stage index %d out of range [0, %d)", stageIndex, len(cs.stages))
	}
	total := len(agents)
	if total == 0 {
		return nil, nil
	}
	stage := cs.stages[stageIndex]
	count := int(math.Round(float64(total) * float64(stage.Percentage) / 100.0))
	if count <= 0 {
		count = 1
	}
	if count > total {
		count = total
	}
	return agents[:count], nil
}
