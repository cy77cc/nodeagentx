package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config is the root runtime configuration.
type Config struct {
	Agent      AgentConfig      `mapstructure:"agent"`
	Server     ServerConfig     `mapstructure:"server"`
	Executor   ExecutorConfig   `mapstructure:"executor"`
	Reporter   ReporterConfig   `mapstructure:"reporter"`
	Auth       AuthConfig       `mapstructure:"auth"`
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Plugin        PluginConfig        `mapstructure:"plugin"`
	GRPC          GRPCConfig          `mapstructure:"grpc"`
	Sandbox       SandboxConfig       `mapstructure:"sandbox"`
	Collector     CollectorConfig     `mapstructure:"collector"`
	PluginGateway PluginGatewayConfig `mapstructure:"plugin_gateway"`
	Checker       CheckerConfig       `mapstructure:"checker"`
	Gateway       GatewayConfig       `mapstructure:"gateway"`
	Tracing       TracingConfig       `mapstructure:"tracing"`
	Alerting      AlertingConfig      `mapstructure:"alerting"`
	Discovery     DiscoveryConfig     `mapstructure:"discovery"`
	Updater       UpdaterConfig       `mapstructure:"updater"`
	WASM          WASMConfig          `mapstructure:"wasm"`
	Federation    FederationConfig    `mapstructure:"federation"`
}

// AgentConfig controls agent identity and collection cadence.
type AgentConfig struct {
	ID                     string          `mapstructure:"id"`
	Name                   string          `mapstructure:"name"`
	Mode                   string          `mapstructure:"mode"` // standalone | hub | leaf
	IntervalSeconds        int             `mapstructure:"interval_seconds"`
	ShutdownTimeoutSeconds int             `mapstructure:"shutdown_timeout_seconds"`
	AuditLog               AuditLogConfig  `mapstructure:"audit_log"`
}

// AuditLogConfig controls the agent-level audit log.
type AuditLogConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
}

// ServerConfig controls local API server settings.
type ServerConfig struct {
	ListenAddr string `mapstructure:"listen_addr"`
}

// ExecutorConfig controls command execution boundaries.
type ExecutorConfig struct {
	TimeoutSeconds  int      `mapstructure:"timeout_seconds"`
	AllowedCommands []string `mapstructure:"allowed_commands"`
	MaxOutputBytes  int      `mapstructure:"max_output_bytes"`
}

// ReporterConfig controls how data is reported.
type ReporterConfig struct {
	Mode            string `mapstructure:"mode"`
	Endpoint        string `mapstructure:"endpoint"`
	TimeoutSeconds  int    `mapstructure:"timeout_seconds"`
	RetryCount      int    `mapstructure:"retry_count"`
	RetryIntervalMS int    `mapstructure:"retry_interval_ms"`
}

// AuthConfig controls API authentication.
type AuthConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	BearerToken string `mapstructure:"bearer_token"`
}

// PrometheusConfig controls exporter endpoint behavior.
type PrometheusConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Path            string `mapstructure:"path"`
	ProtectWithAuth bool   `mapstructure:"protect_with_auth"`
}

// PluginConfig controls rust runtime integration.
type PluginConfig struct {
	Enabled               bool   `mapstructure:"enabled"`
	RuntimePath           string `mapstructure:"runtime_path"`
	SocketPath            string `mapstructure:"socket_path"`
	AutoStart             bool   `mapstructure:"auto_start"`
	StartupTimeoutSeconds int    `mapstructure:"startup_timeout_seconds"`
	RequestTimeoutSeconds int    `mapstructure:"request_timeout_seconds"`
	MaxConcurrentTasks    int    `mapstructure:"max_concurrent_tasks"`
	MaxResultBytes        int    `mapstructure:"max_result_bytes"`
	ChunkSizeBytes        int    `mapstructure:"chunk_size_bytes"`
	SandboxProfile        string `mapstructure:"sandbox_profile"`
}

// GRPCConfig controls the gRPC client connection to the platform.
type GRPCConfig struct {
	ServerAddr                string     `mapstructure:"server_addr"`
	EnrollToken               string     `mapstructure:"enroll_token"`
	MTLS                      MTLSConfig `mapstructure:"mtls"`
	HeartbeatIntervalSeconds  int        `mapstructure:"heartbeat_interval_seconds"`
	ReconnectInitialBackoffMS int        `mapstructure:"reconnect_initial_backoff_ms"`
	ReconnectMaxBackoffMS     int        `mapstructure:"reconnect_max_backoff_ms"`
	CachePersistPath          string     `mapstructure:"cache_persist_path"`
}

// MTLSConfig holds mutual TLS certificate paths.
type MTLSConfig struct {
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
	CAFile   string `mapstructure:"ca_file"`
}

// SandboxConfig controls nsjail sandbox execution.
type SandboxConfig struct {
	Enabled                  bool         `mapstructure:"enabled"`
	NsjailPath               string       `mapstructure:"nsjail_path"`
	BaseWorkdir              string       `mapstructure:"base_workdir"`
	DefaultTimeoutSeconds    int          `mapstructure:"default_timeout_seconds"`
	MaxConcurrentTasks       int          `mapstructure:"max_concurrent_tasks"`
	CgroupBasePath           string       `mapstructure:"cgroup_base_path"`
	AuditLogPath             string       `mapstructure:"audit_log_path"`
	Policy                   PolicyConfig `mapstructure:"policy"`
	AllowUnsandboxedFallback bool         `mapstructure:"allow_unsandboxed_fallback"`
}

// PolicyConfig defines the sandbox security policy.
type PolicyConfig struct {
	AllowedCommands     []string `mapstructure:"allowed_commands"`
	BlockedCommands     []string `mapstructure:"blocked_commands"`
	BlockedKeywords     []string `mapstructure:"blocked_keywords"`
	AllowedInterpreters []string `mapstructure:"allowed_interpreters"`
	ScriptMaxBytes      int      `mapstructure:"script_max_bytes"`
	ShellInjectionCheck bool     `mapstructure:"shell_injection_check"`
}

// CollectorConfig defines the metric collection pipeline.
type CollectorConfig struct {
	Inputs      []PluginInstanceConfig `mapstructure:"inputs"`
	Processors  []PluginInstanceConfig `mapstructure:"processors"`
	Aggregators []PluginInstanceConfig `mapstructure:"aggregators"`
	Outputs     []PluginInstanceConfig `mapstructure:"outputs"`
}

// PluginInstanceConfig is a single plugin instance in the collector pipeline.
type PluginInstanceConfig struct {
	Type   string                 `mapstructure:"type"`
	Config map[string]interface{} `mapstructure:"config"`
}

// PluginGatewayConfig manages custom plugin discovery and lifecycle.
type PluginGatewayConfig struct {
	Enabled                 bool                              `mapstructure:"enabled"`
	PluginsDir              string                            `mapstructure:"plugins_dir"`
	StartupTimeoutSeconds   int                               `mapstructure:"startup_timeout_seconds"`
	HealthCheckIntervalSecs int                               `mapstructure:"health_check_interval_seconds"`
	MaxRestarts             int                               `mapstructure:"max_restarts"`
	RestartBackoffSeconds   int                               `mapstructure:"restart_backoff_seconds"`
	FileWatchDebounceSecs   int                               `mapstructure:"file_watch_debounce_seconds"`
	PluginConfigs           map[string]map[string]interface{} `mapstructure:"plugin_configs"`
}

// CheckerConfig controls the system health checker subsystem.
type CheckerConfig struct {
	Enabled               bool     `mapstructure:"enabled"`
	MaxConcurrent         int      `mapstructure:"max_concurrent"`
	DefaultTimeoutSeconds int      `mapstructure:"default_timeout_seconds"`
	DisabledCheckers      []string `mapstructure:"disabled_checkers"`
}

// GatewayConfig controls the gateway/tunnel subsystem.
type GatewayConfig struct {
	Enabled              bool                `mapstructure:"enabled"`
	ListenAddr           string              `mapstructure:"listen_addr"`
	MaxTunnels           int                 `mapstructure:"max_tunnels"`
	TunnelTimeoutSeconds int                 `mapstructure:"tunnel_timeout_seconds"`
	IdleTimeoutSeconds   int                 `mapstructure:"idle_timeout_seconds"`
	Hosts                []GatewayHostConfig `mapstructure:"hosts"`
}

// GatewayHostConfig defines an internal host behind this gateway.
type GatewayHostConfig struct {
	ID   string           `mapstructure:"id"`
	Addr string           `mapstructure:"addr"`
	Mode string           `mapstructure:"mode"` // "tunnel", "proxy", "auto"
	SSH  GatewaySSHConfig `mapstructure:"ssh"`
}

// GatewaySSHConfig holds SSH credentials for proxy mode.
type GatewaySSHConfig struct {
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	KeyFile  string `mapstructure:"key_file"`
	Port     int    `mapstructure:"port"`
}

// TracingConfig controls distributed tracing subsystem.
type TracingConfig struct {
	Enabled   bool             `mapstructure:"enabled"`
	Receiver  TracingReceiver  `mapstructure:"receiver"`
	Processor TracingProcessor `mapstructure:"processor"`
	Exporter  TracingExporter  `mapstructure:"exporter"`
}

// TracingReceiver configures the trace data receiver.
type TracingReceiver struct {
	GRPCAddr string `mapstructure:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr"`
}

// TracingProcessor configures trace batch processing.
type TracingProcessor struct {
	BatchTimeoutMs int `mapstructure:"batch_timeout_ms"`
	MaxBatchSize   int `mapstructure:"max_batch_size"`
}

// TracingExporter configures where traces are sent.
type TracingExporter struct {
	Endpoint string `mapstructure:"endpoint"`
	Protocol string `mapstructure:"protocol"`
}

// AlertingConfig controls the alerting subsystem.
type AlertingConfig struct {
	Enabled bool        `mapstructure:"enabled"`
	Rules   []AlertRule `mapstructure:"rules"`
}

// AlertRule defines a single alerting rule.
type AlertRule struct {
	Name      string         `mapstructure:"name"`
	Condition AlertCondition `mapstructure:"condition"`
	Severity  string         `mapstructure:"severity"`
	Notify    []AlertNotify  `mapstructure:"notify"`
}

// AlertCondition defines when an alert fires.
type AlertCondition struct {
	Metric    string  `mapstructure:"metric"`
	Operator  string  `mapstructure:"operator"`
	Threshold float64 `mapstructure:"threshold"`
	For       string  `mapstructure:"for"`
}

// AlertNotify defines how alert notifications are sent.
type AlertNotify struct {
	Type    string            `mapstructure:"type"`
	URL     string            `mapstructure:"url"`
	Headers map[string]string `mapstructure:"headers"`
}

// DiscoveryConfig controls the service discovery subsystem.
type DiscoveryConfig struct {
	Enabled     bool                   `mapstructure:"enabled"`
	IntervalSec int                    `mapstructure:"interval_seconds"`
	Layers      []DiscoveryLayerConfig `mapstructure:"layers"`
}

// DiscoveryLayerConfig defines a single discovery layer.
type DiscoveryLayerConfig struct {
	Type    string `mapstructure:"type"`
	Enabled bool   `mapstructure:"enabled"`
}

// UpdaterConfig controls the auto-update subsystem.
type UpdaterConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	CurrentPath string `mapstructure:"current_path"`
	BackupPath  string `mapstructure:"backup_path"`
	DownloadDir string `mapstructure:"download_dir"`
}

// WASMConfig controls the WebAssembly plugin runtime.
type WASMConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	PluginsDir string `mapstructure:"plugins_dir"`
	MaxModules int    `mapstructure:"max_modules"`
	CacheDir   string `mapstructure:"cache_dir"`
}

// Agent mode constants.
const (
	AgentModeStandalone = "standalone"
	AgentModeHub        = "hub"
	AgentModeLeaf       = "leaf"
)

// FederationConfig controls multi-cluster federation.
type FederationConfig struct {
	Enabled bool                 `mapstructure:"enabled"`
	Hub     FederationHubConfig  `mapstructure:"hub"`
	Leaf    FederationLeafConfig `mapstructure:"leaf"`
}

// FederationHubConfig controls hub-mode federation settings.
type FederationHubConfig struct {
	ListenAddr                    string                 `mapstructure:"listen_addr"`
	Region                        string                 `mapstructure:"region"`
	MaxLeaves                     int                    `mapstructure:"max_leaves"`
	LeafHeartbeatTimeoutSeconds   int                    `mapstructure:"leaf_heartbeat_timeout_seconds"`
	MetricsAggregationIntervalSec int                    `mapstructure:"metrics_aggregation_interval_seconds"`
	Security                      FederationSecurity     `mapstructure:"security"`
	Groups                        []GroupRuleConfig      `mapstructure:"groups"`
	ConfigLevels                  ConfigLevelsConfig     `mapstructure:"config_levels"`
	Canary                        CanaryConfig           `mapstructure:"canary"`
	Operations                    OperationsConfig       `mapstructure:"operations"`
}

// FederationLeafConfig controls leaf-mode federation settings.
type FederationLeafConfig struct {
	HubAddr              string         `mapstructure:"hub_addr"`
	ReconnectIntervalSec int            `mapstructure:"reconnect_interval_seconds"`
	ReportIntervalSec    int            `mapstructure:"report_interval_seconds"`
	Fallback             FallbackConfig `mapstructure:"fallback"`
}

// FederationSecurity holds federation security configuration.
type FederationSecurity struct {
	MTLS MTLSConfig `mapstructure:"mtls"`
	PSK  string     `mapstructure:"psk"`
}

// GroupRuleConfig defines a leaf grouping rule.
type GroupRuleConfig struct {
	Name  string            `mapstructure:"name"`
	Match map[string]string `mapstructure:"match"`
}

// ConfigLevelsConfig defines hierarchical configuration levels.
type ConfigLevelsConfig struct {
	Global  map[string]interface{}            `mapstructure:"global"`
	Regions map[string]map[string]interface{} `mapstructure:"regions"`
	Groups  map[string]map[string]interface{} `mapstructure:"groups"`
	Agents  map[string]map[string]interface{} `mapstructure:"agents"`
}

// CanaryConfig controls canary deployment settings.
type CanaryConfig struct {
	Strategy string              `mapstructure:"strategy"`
	Stages   []CanaryStageConfig `mapstructure:"stages"`
}

// CanaryStageConfig defines a single canary stage.
type CanaryStageConfig struct {
	Percentage   int  `mapstructure:"percentage"`
	WaitSeconds  int  `mapstructure:"wait_seconds"`
	AutoRollback bool `mapstructure:"auto_rollback"`
}

// OperationsConfig controls federation operations.
type OperationsConfig struct {
	MaxConcurrent         int               `mapstructure:"max_concurrent"`
	DefaultTimeoutSeconds int               `mapstructure:"default_timeout_seconds"`
	RetryPolicy           RetryPolicyConfig `mapstructure:"retry_policy"`
}

// RetryPolicyConfig controls retry behavior.
type RetryPolicyConfig struct {
	MaxRetries     int `mapstructure:"max_retries"`
	BackoffSeconds int `mapstructure:"backoff_seconds"`
}

// FallbackConfig controls leaf fallback behavior.
type FallbackConfig struct {
	Enabled          bool   `mapstructure:"enabled"`
	Mode             string `mapstructure:"mode"`
	PlatformAddr     string `mapstructure:"platform_addr"`
	CheckIntervalSec int    `mapstructure:"check_interval_seconds"`
}

// Load reads and validates configuration from a file path.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("agent.interval_seconds", 10)
	v.SetDefault("agent.shutdown_timeout_seconds", 30)
	v.SetDefault("agent.audit_log.enabled", false)
	v.SetDefault("agent.audit_log.path", "/var/log/opsagent/audit.jsonl")
	v.SetDefault("agent.audit_log.max_size_mb", 100)
	v.SetDefault("agent.audit_log.max_backups", 5)
	v.SetDefault("server.listen_addr", "127.0.0.1:18080")
	v.SetDefault("executor.timeout_seconds", 10)
	v.SetDefault("executor.max_output_bytes", 65536)
	v.SetDefault("reporter.mode", "stdout")
	v.SetDefault("reporter.timeout_seconds", 5)
	v.SetDefault("reporter.retry_count", 3)
	v.SetDefault("reporter.retry_interval_ms", 500)
	v.SetDefault("auth.enabled", true)
	v.SetDefault("prometheus.enabled", true)
	v.SetDefault("prometheus.path", "/metrics")
	v.SetDefault("prometheus.protect_with_auth", false)
	v.SetDefault("plugin.enabled", false)
	v.SetDefault("plugin.runtime_path", "./rust-runtime/target/release/github.com/cy77cc/opsagent-rust-runtime")
	v.SetDefault("plugin.socket_path", "/tmp/github.com/cy77cc/opsagent/plugin.sock")
	v.SetDefault("plugin.auto_start", true)
	v.SetDefault("plugin.startup_timeout_seconds", 5)
	v.SetDefault("plugin.request_timeout_seconds", 30)
	v.SetDefault("plugin.max_concurrent_tasks", 4)
	v.SetDefault("plugin.max_result_bytes", 8388608)
	v.SetDefault("plugin.chunk_size_bytes", 262144)
	v.SetDefault("plugin.sandbox_profile", "strict")
	v.SetDefault("grpc.heartbeat_interval_seconds", 15)
	v.SetDefault("grpc.reconnect_initial_backoff_ms", 1000)
	v.SetDefault("grpc.reconnect_max_backoff_ms", 30000)
	v.SetDefault("grpc.cache_persist_path", "")
	v.SetDefault("sandbox.enabled", false)
	v.SetDefault("sandbox.default_timeout_seconds", 30)
	v.SetDefault("sandbox.max_concurrent_tasks", 4)
	v.SetDefault("sandbox.policy.shell_injection_check", true)
	v.SetDefault("sandbox.policy.script_max_bytes", 65536)
	v.SetDefault("plugin_gateway.enabled", false)
	v.SetDefault("plugin_gateway.plugins_dir", "/etc/opsagent/plugins")
	v.SetDefault("plugin_gateway.startup_timeout_seconds", 10)
	v.SetDefault("plugin_gateway.health_check_interval_seconds", 30)
	v.SetDefault("plugin_gateway.max_restarts", 3)
	v.SetDefault("plugin_gateway.restart_backoff_seconds", 5)
	v.SetDefault("plugin_gateway.file_watch_debounce_seconds", 2)
	v.SetDefault("checker.enabled", true)
	v.SetDefault("checker.max_concurrent", 5)
	v.SetDefault("checker.default_timeout_seconds", 30)
	v.SetDefault("gateway.enabled", false)
	v.SetDefault("gateway.listen_addr", ":18081")
	v.SetDefault("gateway.max_tunnels", 100)
	v.SetDefault("gateway.tunnel_timeout_seconds", 30)
	v.SetDefault("gateway.idle_timeout_seconds", 300)
	v.SetDefault("tracing.enabled", false)
	v.SetDefault("tracing.receiver.grpc_addr", "0.0.0.0:4317")
	v.SetDefault("tracing.receiver.http_addr", "0.0.0.0:4318")
	v.SetDefault("tracing.processor.batch_timeout_ms", 5000)
	v.SetDefault("tracing.processor.max_batch_size", 512)
	v.SetDefault("tracing.exporter.protocol", "grpc")
	v.SetDefault("alerting.enabled", false)
	v.SetDefault("discovery.enabled", false)
	v.SetDefault("discovery.interval_seconds", 300)
	v.SetDefault("updater.enabled", false)
	v.SetDefault("wasm.enabled", false)
	v.SetDefault("wasm.plugins_dir", "/etc/opsagent/wasm-plugins")
	v.SetDefault("wasm.max_modules", 10)
	v.SetDefault("wasm.cache_dir", "/var/lib/opsagent/wasm-cache")

	// Federation defaults.
	v.SetDefault("agent.mode", AgentModeStandalone)
	v.SetDefault("federation.enabled", false)
	v.SetDefault("federation.hub.listen_addr", ":9443")
	v.SetDefault("federation.hub.max_leaves", 500)
	v.SetDefault("federation.hub.leaf_heartbeat_timeout_seconds", 60)
	v.SetDefault("federation.hub.metrics_aggregation_interval_seconds", 30)
	v.SetDefault("federation.hub.canary.strategy", "percentage")
	v.SetDefault("federation.hub.operations.max_concurrent", 50)
	v.SetDefault("federation.hub.operations.default_timeout_seconds", 300)
	v.SetDefault("federation.hub.operations.retry_policy.max_retries", 3)
	v.SetDefault("federation.hub.operations.retry_policy.backoff_seconds", 10)
	v.SetDefault("federation.leaf.reconnect_interval_seconds", 5)
	v.SetDefault("federation.leaf.report_interval_seconds", 30)
	v.SetDefault("federation.leaf.fallback.enabled", false)
	v.SetDefault("federation.leaf.fallback.mode", "standalone")
	v.SetDefault("federation.leaf.fallback.check_interval_seconds", 30)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := v.UnmarshalExact(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks required config values and sane bounds.
func (c *Config) Validate() error {
	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}
	if c.Agent.Name == "" {
		return fmt.Errorf("agent.name is required")
	}
	if c.Agent.IntervalSeconds <= 0 {
		return fmt.Errorf("agent.interval_seconds must be > 0")
	}
	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr is required")
	}
	if c.Executor.TimeoutSeconds <= 0 {
		return fmt.Errorf("executor.timeout_seconds must be > 0")
	}
	if len(c.Executor.AllowedCommands) == 0 {
		return fmt.Errorf("executor.allowed_commands must not be empty")
	}
	for _, cmd := range c.Executor.AllowedCommands {
		if strings.TrimSpace(cmd) == "" {
			return fmt.Errorf("executor.allowed_commands contains empty command")
		}
	}
	if c.Executor.MaxOutputBytes <= 0 {
		return fmt.Errorf("executor.max_output_bytes must be > 0")
	}

	switch c.Reporter.Mode {
	case "stdout", "http":
	default:
		return fmt.Errorf("reporter.mode must be one of: stdout, http")
	}
	if c.Reporter.Mode == "http" && strings.TrimSpace(c.Reporter.Endpoint) == "" {
		return fmt.Errorf("reporter.endpoint is required when reporter.mode=http")
	}
	if c.Reporter.TimeoutSeconds <= 0 {
		return fmt.Errorf("reporter.timeout_seconds must be > 0")
	}
	if c.Reporter.RetryCount < 0 {
		return fmt.Errorf("reporter.retry_count must be >= 0")
	}
	if c.Reporter.RetryIntervalMS < 0 {
		return fmt.Errorf("reporter.retry_interval_ms must be >= 0")
	}

	if c.Auth.Enabled {
		token := strings.TrimSpace(c.Auth.BearerToken)
		if token == "" {
			return fmt.Errorf("auth.bearer_token is required when auth.enabled=true")
		}
		if len(token) < 32 {
			return fmt.Errorf("auth.bearer_token must be at least 32 characters when auth.enabled=true")
		}
	}

	if c.Prometheus.Enabled {
		if strings.TrimSpace(c.Prometheus.Path) == "" {
			return fmt.Errorf("prometheus.path is required when prometheus.enabled=true")
		}
		if !strings.HasPrefix(c.Prometheus.Path, "/") {
			return fmt.Errorf("prometheus.path must start with /")
		}
	}

	if c.Plugin.Enabled {
		if strings.TrimSpace(c.Plugin.SocketPath) == "" {
			return fmt.Errorf("plugin.socket_path is required when plugin.enabled=true")
		}
		if c.Plugin.AutoStart && strings.TrimSpace(c.Plugin.RuntimePath) == "" {
			return fmt.Errorf("plugin.runtime_path is required when plugin.auto_start=true")
		}
		if c.Plugin.StartupTimeoutSeconds <= 0 {
			return fmt.Errorf("plugin.startup_timeout_seconds must be > 0")
		}
		if c.Plugin.RequestTimeoutSeconds <= 0 {
			return fmt.Errorf("plugin.request_timeout_seconds must be > 0")
		}
		if c.Plugin.MaxConcurrentTasks <= 0 {
			return fmt.Errorf("plugin.max_concurrent_tasks must be > 0")
		}
		if c.Plugin.MaxResultBytes <= 0 {
			return fmt.Errorf("plugin.max_result_bytes must be > 0")
		}
		if c.Plugin.ChunkSizeBytes <= 0 {
			return fmt.Errorf("plugin.chunk_size_bytes must be > 0")
		}
		if strings.TrimSpace(c.Plugin.SandboxProfile) == "" {
			return fmt.Errorf("plugin.sandbox_profile is required")
		}
	}

	// GRPC validation.
	if strings.TrimSpace(c.GRPC.ServerAddr) == "" {
		return fmt.Errorf("grpc.server_addr is required")
	}
	if c.GRPC.HeartbeatIntervalSeconds <= 0 {
		return fmt.Errorf("grpc.heartbeat_interval_seconds must be > 0")
	}
	if c.GRPC.ReconnectInitialBackoffMS <= 0 {
		return fmt.Errorf("grpc.reconnect_initial_backoff_ms must be > 0")
	}
	if c.GRPC.ReconnectMaxBackoffMS <= 0 {
		return fmt.Errorf("grpc.reconnect_max_backoff_ms must be > 0")
	}

	// Sandbox validation (only when enabled).
	if c.Sandbox.Enabled {
		if strings.TrimSpace(c.Sandbox.NsjailPath) == "" {
			return fmt.Errorf("sandbox.nsjail_path is required when sandbox.enabled=true")
		}
		if strings.TrimSpace(c.Sandbox.BaseWorkdir) == "" {
			return fmt.Errorf("sandbox.base_workdir is required when sandbox.enabled=true")
		}
		if c.Sandbox.DefaultTimeoutSeconds <= 0 {
			return fmt.Errorf("sandbox.default_timeout_seconds must be > 0")
		}
		if c.Sandbox.MaxConcurrentTasks <= 0 {
			return fmt.Errorf("sandbox.max_concurrent_tasks must be > 0")
		}
		if strings.TrimSpace(c.Sandbox.CgroupBasePath) == "" {
			return fmt.Errorf("sandbox.cgroup_base_path is required when sandbox.enabled=true")
		}
		if strings.TrimSpace(c.Sandbox.AuditLogPath) == "" {
			return fmt.Errorf("sandbox.audit_log_path is required when sandbox.enabled=true")
		}
		if len(c.Sandbox.Policy.AllowedCommands) == 0 {
			return fmt.Errorf("sandbox.policy.allowed_commands must not be empty when sandbox.enabled=true")
		}
		if c.Sandbox.Policy.ScriptMaxBytes <= 0 {
			return fmt.Errorf("sandbox.policy.script_max_bytes must be > 0")
		}
	}

	// PluginGateway validation (only when enabled).
	if c.PluginGateway.Enabled {
		if strings.TrimSpace(c.PluginGateway.PluginsDir) == "" {
			return fmt.Errorf("plugin_gateway.plugins_dir is required when plugin_gateway.enabled=true")
		}
		if c.PluginGateway.StartupTimeoutSeconds <= 0 {
			return fmt.Errorf("plugin_gateway.startup_timeout_seconds must be > 0")
		}
		if c.PluginGateway.HealthCheckIntervalSecs <= 0 {
			return fmt.Errorf("plugin_gateway.health_check_interval_seconds must be > 0")
		}
		if c.PluginGateway.MaxRestarts < 0 {
			return fmt.Errorf("plugin_gateway.max_restarts must be >= 0")
		}
	}

	// Checker validation (only when enabled).
	if c.Checker.Enabled {
		if c.Checker.MaxConcurrent <= 0 {
			return fmt.Errorf("checker.max_concurrent must be > 0 when checker.enabled=true")
		}
		if c.Checker.DefaultTimeoutSeconds <= 0 {
			return fmt.Errorf("checker.default_timeout_seconds must be > 0 when checker.enabled=true")
		}
	}

	// Gateway validation (only when enabled).
	if c.Gateway.Enabled {
		if strings.TrimSpace(c.Gateway.ListenAddr) == "" {
			return fmt.Errorf("gateway.listen_addr is required when gateway.enabled=true")
		}
		if c.Gateway.MaxTunnels <= 0 {
			return fmt.Errorf("gateway.max_tunnels must be > 0 when gateway.enabled=true")
		}
		if c.Gateway.TunnelTimeoutSeconds <= 0 {
			return fmt.Errorf("gateway.tunnel_timeout_seconds must be > 0 when gateway.enabled=true")
		}
		if c.Gateway.IdleTimeoutSeconds <= 0 {
			return fmt.Errorf("gateway.idle_timeout_seconds must be > 0 when gateway.enabled=true")
		}
		for i, h := range c.Gateway.Hosts {
			if h.ID == "" {
				return fmt.Errorf("gateway.hosts[%d].id is required", i)
			}
			if h.Addr == "" {
				return fmt.Errorf("gateway.hosts[%d].addr is required", i)
			}
			switch h.Mode {
			case "tunnel", "proxy", "auto":
			default:
				return fmt.Errorf("gateway.hosts[%d].mode must be one of: tunnel, proxy, auto", i)
			}
			if h.Mode == "proxy" || h.Mode == "auto" {
				if h.SSH.User == "" {
					return fmt.Errorf("gateway.hosts[%d].ssh.user is required when mode=%s", i, h.Mode)
				}
				if h.SSH.Port <= 0 {
					return fmt.Errorf("gateway.hosts[%d].ssh.port must be > 0 when mode=%s", i, h.Mode)
				}
			}
		}
	}

	// Tracing validation (only when enabled).
	if c.Tracing.Enabled {
		if c.Tracing.Exporter.Endpoint == "" {
			return fmt.Errorf("tracing.exporter.endpoint is required when tracing is enabled")
		}
	}

	// Alerting validation (only when enabled).
	if c.Alerting.Enabled {
		for i, rule := range c.Alerting.Rules {
			if rule.Name == "" {
				return fmt.Errorf("alerting.rules[%d].name is required", i)
			}
			if rule.Condition.Metric == "" {
				return fmt.Errorf("alerting.rules[%d].condition.metric is required", i)
			}
			validOps := map[string]bool{">": true, "<": true, ">=": true, "<=": true, "==": true, "!=": true}
			if !validOps[rule.Condition.Operator] {
				return fmt.Errorf("alerting.rules[%d].condition.operator must be one of: >, <, >=, <=, ==, !=", i)
			}
		}
	}

	// Federation validation (only when enabled and in hub/leaf mode).
	if c.Federation.Enabled && (c.Agent.Mode == AgentModeHub || c.Agent.Mode == AgentModeLeaf) {
		switch c.Agent.Mode {
		case AgentModeHub:
			if strings.TrimSpace(c.Federation.Hub.ListenAddr) == "" {
				return fmt.Errorf("federation.hub.listen_addr is required when agent.mode=hub")
			}
			if strings.TrimSpace(c.Federation.Hub.Region) == "" {
				return fmt.Errorf("federation.hub.region is required when agent.mode=hub")
			}
			if c.Federation.Hub.MaxLeaves <= 0 {
				return fmt.Errorf("federation.hub.max_leaves must be > 0 when agent.mode=hub")
			}
			if c.Federation.Hub.LeafHeartbeatTimeoutSeconds <= 0 {
				return fmt.Errorf("federation.hub.leaf_heartbeat_timeout_seconds must be > 0 when agent.mode=hub")
			}
			if c.Federation.Hub.MetricsAggregationIntervalSec <= 0 {
				return fmt.Errorf("federation.hub.metrics_aggregation_interval_seconds must be > 0 when agent.mode=hub")
			}
			for i, stage := range c.Federation.Hub.Canary.Stages {
				if stage.Percentage <= 0 || stage.Percentage > 100 {
					return fmt.Errorf("federation.hub.canary.stages[%d].percentage must be 1-100", i)
				}
				if i > 0 && stage.Percentage <= c.Federation.Hub.Canary.Stages[i-1].Percentage {
					return fmt.Errorf("federation.hub.canary.stages must be sorted by percentage ascending")
				}
			}
			switch c.Federation.Hub.Canary.Strategy {
			case "", "percentage":
			default:
				return fmt.Errorf("federation.hub.canary.strategy must be one of: percentage")
			}
		case AgentModeLeaf:
			if strings.TrimSpace(c.Federation.Leaf.HubAddr) == "" {
				return fmt.Errorf("federation.leaf.hub_addr is required when agent.mode=leaf")
			}
			switch c.Federation.Leaf.Fallback.Mode {
			case "", "standalone":
			default:
				return fmt.Errorf("federation.leaf.fallback.mode must be one of: standalone")
			}
		}
	}

	return nil
}
