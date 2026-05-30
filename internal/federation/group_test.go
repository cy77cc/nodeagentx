package federation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupEngine_Evaluate_MatchesExactLabels(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod-web", Match: map[string]string{"env": "prod", "role": "web"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod", "role": "web"}}
	groups := ge.Evaluate(leaf)
	assert.Equal(t, []string{"prod-web"}, groups)
}

func TestGroupEngine_Evaluate_NoMatch(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod-web", Match: map[string]string{"env": "prod", "role": "web"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "staging", "role": "db"}}
	groups := ge.Evaluate(leaf)
	assert.Empty(t, groups)
}

func TestGroupEngine_Evaluate_MultipleGroups(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
		{Name: "web", Match: map[string]string{"role": "web"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod", "role": "web"}}
	groups := ge.Evaluate(leaf)
	assert.Contains(t, groups, "prod")
	assert.Contains(t, groups, "web")
	assert.Len(t, groups, 2)
}

func TestGroupEngine_Evaluate_PartialMatch(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod-web", Match: map[string]string{"env": "prod", "role": "web"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod", "role": "db"}}
	groups := ge.Evaluate(leaf)
	assert.Empty(t, groups)
}

func TestGroupEngine_UpdateLeaf_UpdatesGroups(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod"}, LastSeen: time.Now()}
	ge.UpdateLeaf(leaf)
	members := ge.GetGroupMembers("prod")
	assert.Equal(t, []string{"agent-001"}, members)
}

func TestGroupEngine_RemoveLeaf(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod"}, LastSeen: time.Now()}
	ge.UpdateLeaf(leaf)
	ge.RemoveLeaf("agent-001")
	members := ge.GetGroupMembers("prod")
	assert.Empty(t, members)
}

func TestGroupEngine_GetGroupMembers_EmptyGroup(t *testing.T) {
	ge := NewGroupEngine(nil)
	members := ge.GetGroupMembers("nonexistent")
	assert.Empty(t, members)
}

func TestGroupEngine_UpdateLeaf_ReEvaluatesOnLabelChange(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
		{Name: "staging", Match: map[string]string{"env": "staging"}},
	})
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod"}, LastSeen: time.Now()}
	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("prod"), "agent-001")

	leaf.Labels = map[string]string{"env": "staging"}
	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("staging"), "agent-001")
	assert.NotContains(t, ge.GetGroupMembers("prod"), "agent-001")
}

func TestGroupEngine_GetAllLeaves(t *testing.T) {
	ge := NewGroupEngine(nil)
	leaf := &LeafState{AgentID: "agent-001", Labels: map[string]string{"env": "prod"}, LastSeen: time.Now()}
	ge.UpdateLeaf(leaf)
	all := ge.GetAllLeaves()
	require.Len(t, all, 1)
	assert.Equal(t, "agent-001", all["agent-001"].AgentID)
}
