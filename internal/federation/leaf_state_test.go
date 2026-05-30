package federation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLeafState_IsOnline_WhenRecentlySeen(t *testing.T) {
	ls := &LeafState{AgentID: "agent-001", LastSeen: time.Now()}
	assert.True(t, ls.IsOnline(60*time.Second))
}

func TestLeafState_IsOnline_WhenStale(t *testing.T) {
	ls := &LeafState{AgentID: "agent-001", LastSeen: time.Now().Add(-120 * time.Second)}
	assert.False(t, ls.IsOnline(60 * time.Second))
}

func TestLeafState_AllLabels_MergesManualAndAuto(t *testing.T) {
	ls := &LeafState{
		AgentID:   "agent-001",
		Labels:     map[string]string{"env": "prod", "role": "web"},
		AutoLabels: map[string]string{"os": "linux", "arch": "amd64"},
	}
	all := ls.AllLabels()
	assert.Equal(t, "prod", all["env"])
	assert.Equal(t, "linux", all["os"])
}

func TestLeafState_AllLabels_AutoOverriddenByManual(t *testing.T) {
	ls := &LeafState{
		AgentID:   "agent-001",
		Labels:     map[string]string{"os": "custom-value"},
		AutoLabels: map[string]string{"os": "linux"},
	}
	all := ls.AllLabels()
	assert.Equal(t, "custom-value", all["os"])
}
