package federation

// Stub types for federation protocol messages.
// These mirror the proto-generated types in the proto/ sub-package but live
// in the federation package so that in-process components (HubServer, LeafClient)
// can use them without importing the proto package directly.

// FedAgentRegistration is sent by a Leaf agent to register with the Hub.
type FedAgentRegistration struct {
	AgentId    string            `json:"agent_id"`
	Hostname   string            `json:"hostname"`
	Ip         string            `json:"ip"`
	Version    string            `json:"version"`
	Labels     map[string]string `json:"labels"`
	AutoLabels map[string]string `json:"auto_labels"`
}

// FedRegisterResponse is returned by the Hub after a Leaf registers.
type FedRegisterResponse struct {
	Accepted        bool     `json:"accepted"`
	AssignedRegion  string   `json:"assigned_region"`
	AssignedGroups  []string `json:"assigned_groups"`
	ConfigVersion   string   `json:"config_version"`
	RejectionReason string   `json:"rejection_reason,omitzero"`
}

// FedHeartbeatRequest is sent periodically by a Leaf to the Hub.
type FedHeartbeatRequest struct {
	AgentId string            `json:"agent_id"`
	Labels  map[string]string `json:"labels,omitzero"`
}

// FedHeartbeatResponse is returned by the Hub after a heartbeat.
type FedHeartbeatResponse struct {
	Ok                    bool   `json:"ok"`
	ConfigVersion         string `json:"config_version,omitzero"`
	ConfigUpdateAvailable bool   `json:"config_update_available"`
}

// FedConfigUpdate is pushed from Hub to Leaf when configuration changes.
type FedConfigUpdate struct {
	Version string         `json:"version"`
	Config  map[string]any `json:"config"`
}

// FallbackConfig controls Leaf fallback behavior when Hub is unreachable.
type FallbackConfig struct {
	Enabled          bool   `json:"enabled"`
	Mode             string `json:"mode"`
	PlatformAddr     string `json:"platform_addr"`
	CheckIntervalSec int    `json:"check_interval_sec"`
}
