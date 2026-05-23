package gateway

import "time"

// Config holds gateway module configuration.
type Config struct {
	ListenAddr    string
	MaxTunnels    int
	TunnelTimeout time.Duration
	IdleTimeout   time.Duration
	Hosts         []HostConfig
}

// HostConfig defines an internal host.
type HostConfig struct {
	ID   string
	Addr string
	Mode string // "tunnel", "proxy", "auto"
	SSH  SSHConfig
}

// SSHConfig holds SSH connection credentials.
type SSHConfig struct {
	User     string
	Password string
	KeyFile  string
	Port     int
}
