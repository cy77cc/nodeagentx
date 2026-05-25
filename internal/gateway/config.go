package gateway

import "time"

// Config holds gateway module configuration.
type Config struct {
	ListenAddr    string
	MaxTunnels    int
	TunnelTimeout time.Duration
	IdleTimeout   time.Duration
	Hosts         []HostConfig
	AuthPSK       string // pre-shared key for tunnel authentication (empty = no auth)
}

// HostConfig defines an internal host.
type HostConfig struct {
	ID       string
	Hostname string // display name for registration; falls back to ID if empty
	Addr     string
	Mode     string // "tunnel", "proxy", "auto"
	SSH      SSHConfig
}

// SSHConfig holds SSH connection credentials.
type SSHConfig struct {
	User     string
	Password string
	KeyFile  string
	Port     int
}
