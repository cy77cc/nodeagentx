package federation

import "maps"

import "time"

const (
	LeafStatusOnline   = "online"
	LeafStatusOffline  = "offline"
	LeafStatusDegraded = "degraded"
)

// LeafState represents the current state of a connected Leaf agent.
type LeafState struct {
	AgentID       string            `json:"agent_id"`
	Hostname      string            `json:"hostname"`
	IP            string            `json:"ip"`
	Version       string            `json:"version"`
	Labels        map[string]string `json:"labels"`
	AutoLabels    map[string]string `json:"auto_labels"`
	Groups        []string          `json:"groups"`
	LastSeen      time.Time         `json:"last_seen"`
	Status        string            `json:"status"`
	ConfigVersion string            `json:"config_version"`
}

// IsOnline returns true if the Leaf was seen within the given timeout duration.
func (ls *LeafState) IsOnline(timeout time.Duration) bool {
	return time.Since(ls.LastSeen) <= timeout
}

// AllLabels returns a merged view of auto labels and manual labels.
// Manual labels override auto labels when keys conflict.
func (ls *LeafState) AllLabels() map[string]string {
	result := make(map[string]string, len(ls.AutoLabels)+len(ls.Labels))
	maps.Copy(result, ls.AutoLabels)
	maps.Copy(result, ls.Labels)
	return result
}
