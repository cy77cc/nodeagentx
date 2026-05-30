package federation

import (
	"sort"
	"sync"
)

// GroupRule defines a rule for dynamically grouping Leaf agents by label matching.
type GroupRule struct {
	Name  string            `json:"name"`
	Match map[string]string `json:"match"`
}

// GroupEngine manages dynamic grouping of Leaf agents based on label rules.
type GroupEngine struct {
	mu         sync.RWMutex
	rules      []GroupRule
	leafStates map[string]*LeafState
	groupIndex map[string]map[string]bool
}

// NewGroupEngine creates a new GroupEngine with the given rules.
func NewGroupEngine(rules []GroupRule) *GroupEngine {
	return &GroupEngine{
		rules:      rules,
		leafStates: make(map[string]*LeafState),
		groupIndex: make(map[string]map[string]bool),
	}
}

// Evaluate returns the list of group names that a Leaf matches based on its labels.
func (ge *GroupEngine) Evaluate(leaf *LeafState) []string {
	var groups []string
	allLabels := leaf.AllLabels()
	for _, rule := range ge.rules {
		if matchesAll(allLabels, rule.Match) {
			groups = append(groups, rule.Name)
		}
	}
	return groups
}

// UpdateLeaf re-evaluates a Leaf's group membership and updates the index.
func (ge *GroupEngine) UpdateLeaf(leaf *LeafState) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	oldGroups := ge.leafGroups(leaf.AgentID)
	newGroups := ge.Evaluate(leaf)

	// Remove from groups no longer matched
	for _, g := range oldGroups {
		if !contains(newGroups, g) {
			delete(ge.groupIndex[g], leaf.AgentID)
			if len(ge.groupIndex[g]) == 0 {
				delete(ge.groupIndex, g)
			}
		}
	}

	// Add to new groups
	for _, g := range newGroups {
		if ge.groupIndex[g] == nil {
			ge.groupIndex[g] = make(map[string]bool)
		}
		ge.groupIndex[g][leaf.AgentID] = true
	}

	leaf.Groups = newGroups
	ge.leafStates[leaf.AgentID] = leaf
}

// RemoveLeaf removes a Leaf from all groups and the state map.
func (ge *GroupEngine) RemoveLeaf(agentID string) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	leaf, ok := ge.leafStates[agentID]
	if !ok {
		return
	}

	for _, g := range leaf.Groups {
		if ge.groupIndex[g] != nil {
			delete(ge.groupIndex[g], agentID)
			if len(ge.groupIndex[g]) == 0 {
				delete(ge.groupIndex, g)
			}
		}
	}
	delete(ge.leafStates, agentID)
}

// GetGroupMembers returns sorted agent IDs belonging to a group.
func (ge *GroupEngine) GetGroupMembers(groupName string) []string {
	ge.mu.RLock()
	defer ge.mu.RUnlock()

	members := ge.groupIndex[groupName]
	if len(members) == 0 {
		return nil
	}
	ids := make([]string, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GetLeaf returns the state of a specific Leaf agent.
func (ge *GroupEngine) GetLeaf(agentID string) *LeafState {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	return ge.leafStates[agentID]
}

// GetAllLeaves returns a copy of all Leaf states.
func (ge *GroupEngine) GetAllLeaves() map[string]*LeafState {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	result := make(map[string]*LeafState, len(ge.leafStates))
	for k, v := range ge.leafStates {
		result[k] = v
	}
	return result
}

func (ge *GroupEngine) leafGroups(agentID string) []string {
	var groups []string
	for g, members := range ge.groupIndex {
		if members[agentID] {
			groups = append(groups, g)
		}
	}
	return groups
}

func matchesAll(labels, match map[string]string) bool {
	for k, v := range match {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
