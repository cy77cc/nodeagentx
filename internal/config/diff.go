package config

import (
	"fmt"
	"reflect"
)

// ChangeSet records which reloadable field groups changed.
type ChangeSet struct {
	CollectorChanged     bool
	ReporterChanged      bool
	AuthChanged          bool
	PrometheusChanged    bool
	PluginGatewayChanged bool
	CheckerChanged       bool
	AlertingChanged      bool
}

// NonReloadableChange records a change to a field that requires restart.
type NonReloadableChange struct {
	Field  string
	OldVal any
	NewVal any
}

// Diff compares old and new configs, returning a ChangeSet for reloadable
// fields and a list of non-reloadable changes. The new config is validated first.
func Diff(old, new *Config) (*ChangeSet, []NonReloadableChange, error) {
	if err := new.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid new config: %w", err)
	}

	cs := &ChangeSet{}
	var nonReloadable []NonReloadableChange

	// Reloadable: collector
	if !reflect.DeepEqual(old.Collector, new.Collector) {
		cs.CollectorChanged = true
	}

	// Reloadable: reporter
	if diffReporter(old, new) {
		cs.ReporterChanged = true
	}

	// Reloadable: auth
	if diffAuth(old, new) {
		cs.AuthChanged = true
	}

	// Reloadable: prometheus
	if diffPrometheus(old, new) {
		cs.PrometheusChanged = true
	}

	// Reloadable: plugin gateway
	if diffPluginGateway(old, new) {
		cs.PluginGatewayChanged = true
	}

	// Reloadable: checker
	if diffChecker(old, new) {
		cs.CheckerChanged = true
	}

	// Reloadable: alerting
	if diffAlerting(old, new) {
		cs.AlertingChanged = true
	}

	// Non-reloadable checks
	nonReloadable = append(nonReloadable, diffAgent(old, new)...)
	nonReloadable = append(nonReloadable, diffServer(old, new)...)
	nonReloadable = append(nonReloadable, diffGRPC(old, new)...)
	nonReloadable = append(nonReloadable, diffExecutor(old, new)...)

	if !reflect.DeepEqual(old.Sandbox, new.Sandbox) {
		nonReloadable = append(nonReloadable, NonReloadableChange{
			Field:  "sandbox.*",
			OldVal: old.Sandbox,
			NewVal: new.Sandbox,
		})
	}

	if !reflect.DeepEqual(old.Plugin, new.Plugin) {
		nonReloadable = append(nonReloadable, NonReloadableChange{
			Field:  "plugin.*",
			OldVal: old.Plugin,
			NewVal: new.Plugin,
		})
	}

	nonReloadable = append(nonReloadable, diffGateway(old, new)...)

	return cs, nonReloadable, nil
}

// maskSecret returns a masked version of a secret string.
// For strings longer than 4 characters, the first and last 2 characters are shown
// with "***" in between. Shorter strings are fully masked.
func maskSecret(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return s[:2] + "***" + s[len(s)-2:]
}

func diffReporter(old, new *Config) bool {
	return old.Reporter != new.Reporter
}

func diffAuth(old, new *Config) bool {
	return old.Auth != new.Auth
}

func diffPrometheus(old, new *Config) bool {
	return old.Prometheus != new.Prometheus
}

func diffPluginGateway(old, new *Config) bool {
	return !reflect.DeepEqual(old.PluginGateway, new.PluginGateway)
}

func diffChecker(old, new *Config) bool {
	return !reflect.DeepEqual(old.Checker, new.Checker)
}

func diffAlerting(old, new *Config) bool {
	return !reflect.DeepEqual(old.Alerting, new.Alerting)
}

func diffAgent(old, new *Config) []NonReloadableChange {
	var changes []NonReloadableChange
	if old.Agent.ID != new.Agent.ID {
		changes = append(changes, NonReloadableChange{"agent.id", old.Agent.ID, new.Agent.ID})
	}
	if old.Agent.Name != new.Agent.Name {
		changes = append(changes, NonReloadableChange{"agent.name", old.Agent.Name, new.Agent.Name})
	}
	if old.Agent.IntervalSeconds != new.Agent.IntervalSeconds {
		changes = append(changes, NonReloadableChange{"agent.interval_seconds", old.Agent.IntervalSeconds, new.Agent.IntervalSeconds})
	}
	if old.Agent.ShutdownTimeoutSeconds != new.Agent.ShutdownTimeoutSeconds {
		changes = append(changes, NonReloadableChange{"agent.shutdown_timeout_seconds", old.Agent.ShutdownTimeoutSeconds, new.Agent.ShutdownTimeoutSeconds})
	}
	return changes
}

func diffServer(old, new *Config) []NonReloadableChange {
	var changes []NonReloadableChange
	if old.Server.ListenAddr != new.Server.ListenAddr {
		changes = append(changes, NonReloadableChange{"server.listen_addr", old.Server.ListenAddr, new.Server.ListenAddr})
	}
	return changes
}

func diffGRPC(old, new *Config) []NonReloadableChange {
	var changes []NonReloadableChange
	if old.GRPC.ServerAddr != new.GRPC.ServerAddr {
		changes = append(changes, NonReloadableChange{"grpc.server_addr", old.GRPC.ServerAddr, new.GRPC.ServerAddr})
	}
	if old.GRPC.EnrollToken != new.GRPC.EnrollToken {
		changes = append(changes, NonReloadableChange{"grpc.enroll_token", maskSecret(old.GRPC.EnrollToken), maskSecret(new.GRPC.EnrollToken)})
	}
	if old.GRPC.MTLS != new.GRPC.MTLS {
		changes = append(changes, NonReloadableChange{"grpc.mtls", old.GRPC.MTLS, new.GRPC.MTLS})
	}
	if old.GRPC.HeartbeatIntervalSeconds != new.GRPC.HeartbeatIntervalSeconds {
		changes = append(changes, NonReloadableChange{"grpc.heartbeat_interval_seconds", old.GRPC.HeartbeatIntervalSeconds, new.GRPC.HeartbeatIntervalSeconds})
	}
	if old.GRPC.ReconnectInitialBackoffMS != new.GRPC.ReconnectInitialBackoffMS {
		changes = append(changes, NonReloadableChange{"grpc.reconnect_initial_backoff_ms", old.GRPC.ReconnectInitialBackoffMS, new.GRPC.ReconnectInitialBackoffMS})
	}
	if old.GRPC.ReconnectMaxBackoffMS != new.GRPC.ReconnectMaxBackoffMS {
		changes = append(changes, NonReloadableChange{"grpc.reconnect_max_backoff_ms", old.GRPC.ReconnectMaxBackoffMS, new.GRPC.ReconnectMaxBackoffMS})
	}
	if old.GRPC.CachePersistPath != new.GRPC.CachePersistPath {
		changes = append(changes, NonReloadableChange{"grpc.cache_persist_path", old.GRPC.CachePersistPath, new.GRPC.CachePersistPath})
	}
	return changes
}

func diffExecutor(old, new *Config) []NonReloadableChange {
	var changes []NonReloadableChange
	if old.Executor.TimeoutSeconds != new.Executor.TimeoutSeconds {
		changes = append(changes, NonReloadableChange{"executor.timeout_seconds", old.Executor.TimeoutSeconds, new.Executor.TimeoutSeconds})
	}
	if !reflect.DeepEqual(old.Executor.AllowedCommands, new.Executor.AllowedCommands) {
		changes = append(changes, NonReloadableChange{"executor.allowed_commands", old.Executor.AllowedCommands, new.Executor.AllowedCommands})
	}
	if old.Executor.MaxOutputBytes != new.Executor.MaxOutputBytes {
		changes = append(changes, NonReloadableChange{"executor.max_output_bytes", old.Executor.MaxOutputBytes, new.Executor.MaxOutputBytes})
	}
	return changes
}

func diffGateway(old, new *Config) []NonReloadableChange {
	var changes []NonReloadableChange
	if old.Gateway.Enabled != new.Gateway.Enabled {
		changes = append(changes, NonReloadableChange{"gateway.enabled", old.Gateway.Enabled, new.Gateway.Enabled})
	}
	if old.Gateway.ListenAddr != new.Gateway.ListenAddr {
		changes = append(changes, NonReloadableChange{"gateway.listen_addr", old.Gateway.ListenAddr, new.Gateway.ListenAddr})
	}
	if old.Gateway.MaxTunnels != new.Gateway.MaxTunnels {
		changes = append(changes, NonReloadableChange{"gateway.max_tunnels", old.Gateway.MaxTunnels, new.Gateway.MaxTunnels})
	}
	if old.Gateway.TunnelTimeoutSeconds != new.Gateway.TunnelTimeoutSeconds {
		changes = append(changes, NonReloadableChange{"gateway.tunnel_timeout_seconds", old.Gateway.TunnelTimeoutSeconds, new.Gateway.TunnelTimeoutSeconds})
	}
	if old.Gateway.IdleTimeoutSeconds != new.Gateway.IdleTimeoutSeconds {
		changes = append(changes, NonReloadableChange{"gateway.idle_timeout_seconds", old.Gateway.IdleTimeoutSeconds, new.Gateway.IdleTimeoutSeconds})
	}
	// Compare hosts with password masking
	if len(old.Gateway.Hosts) != len(new.Gateway.Hosts) {
		changes = append(changes, NonReloadableChange{"gateway.hosts", len(old.Gateway.Hosts), len(new.Gateway.Hosts)})
	} else {
		for i := range old.Gateway.Hosts {
			oh, nh := old.Gateway.Hosts[i], new.Gateway.Hosts[i]
			if oh.ID != nh.ID || oh.Addr != nh.Addr || oh.Mode != nh.Mode {
				changes = append(changes, NonReloadableChange{fmt.Sprintf("gateway.hosts[%d]", i), oh, nh})
			}
			if oh.SSH.User != nh.SSH.User {
				changes = append(changes, NonReloadableChange{fmt.Sprintf("gateway.hosts[%d].ssh.user", i), oh.SSH.User, nh.SSH.User})
			}
			if oh.SSH.Password != nh.SSH.Password {
				changes = append(changes, NonReloadableChange{fmt.Sprintf("gateway.hosts[%d].ssh.password", i), maskSecret(oh.SSH.Password), maskSecret(nh.SSH.Password)})
			}
			if oh.SSH.KeyFile != nh.SSH.KeyFile {
				changes = append(changes, NonReloadableChange{fmt.Sprintf("gateway.hosts[%d].ssh.key_file", i), oh.SSH.KeyFile, nh.SSH.KeyFile})
			}
			if oh.SSH.Port != nh.SSH.Port {
				changes = append(changes, NonReloadableChange{fmt.Sprintf("gateway.hosts[%d].ssh.port", i), oh.SSH.Port, nh.SSH.Port})
			}
		}
	}
	return changes
}
