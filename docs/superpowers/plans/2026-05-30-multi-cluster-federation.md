# Multi-Cluster Federation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement hierarchical Hub-Leaf federation for OpsAgent, enabling large-scale agent grouping, multi-level config distribution, and batch canary operations.

**Architecture:** OpsAgent gains three operating modes (standalone/hub/leaf) via `agent.mode` config. Hub mode runs a gRPC Server accepting Leaf connections, aggregates metrics, distributes config, and manages batch operations. Leaf mode connects to Hub as a gRPC client with automatic fallback to standalone. Grouping engine dynamically assigns Leaves to groups based on label matching rules.

**Tech Stack:** Go, gRPC, protobuf, existing OpsAgent config/health/audit infrastructure

**Spec:** `docs/superpowers/specs/2026-05-30-multi-cluster-federation-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `internal/config/config.go` | Add `FederationConfig`, `FederationHubConfig`, `FederationLeafConfig` types + validation |
| `internal/config/federation_test.go` | Federation config validation tests |
| `proto/federation.proto` | Federation gRPC service and message definitions |
| `internal/federation/labels.go` | Auto-label collection (os, arch, cloud metadata) |
| `internal/federation/labels_test.go` | Label collection tests |
| `internal/federation/leaf_state.go` | `LeafState` struct and state tracking |
| `internal/federation/group.go` | `GroupEngine` — label matching, dynamic grouping |
| `internal/federation/group_test.go` | Group engine unit tests |
| `internal/federation/config_distributor.go` | Multi-level config inheritance and resolution |
| `internal/federation/config_distributor_test.go` | Config distributor unit tests |
| `internal/federation/server.go` | Hub gRPC Server implementation |
| `internal/federation/client.go` | Leaf gRPC Client implementation |
| `internal/federation/fallback.go` | Leaf fallback to standalone mode |
| `internal/federation/hub.go` | Hub mode orchestrator |
| `internal/federation/canary.go` | Canary/gradual rollout strategy |
| `internal/federation/canary_test.go` | Canary strategy unit tests |
| `internal/federation/operation.go` | Batch operation manager |
| `internal/federation/metrics.go` | Prometheus metrics registration |
| `internal/app/interfaces.go` | Add `FederationHub` and `FederationLeaf` interfaces |
| `internal/app/agent.go` | Wire federation into Agent lifecycle |
| `configs/config.yaml` | Add federation config section |

---

### Task 1: Federation Config Types and Validation

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/federation_test.go`

- [ ] **Step 1: Write failing tests for FederationConfig validation**

```go
// internal/config/federation_test.go
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationConfig_Validation_HubMode_RequiresListenAddr(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.listen_addr")
}

func TestFederationConfig_Validation_HubMode_RequiresRegion(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.region")
}

func TestFederationConfig_Validation_LeafMode_RequiresHubAddr(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = ""

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.leaf.hub_addr")
}

func TestFederationConfig_Validation_HubMode_MaxLeavesPositive(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 0

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "federation.hub.max_leaves")
}

func TestFederationConfig_Validation_HubMode_ValidConfig(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 500
	cfg.Federation.Hub.LeafHeartbeatTimeoutSeconds = 60

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_LeafMode_ValidConfig(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "leaf"
	cfg.Federation.Enabled = true
	cfg.Federation.Leaf.HubAddr = "hub.example.com:9443"

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_DisabledFederation_SkipsChecks(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = false
	// Missing required fields, but federation is disabled

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_StandaloneMode_SkipsChecks(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "standalone"
	cfg.Federation.Enabled = true
	// Missing required fields, but mode is standalone

	err := cfg.Validate()
	assert.NoError(t, err)
}

func TestFederationConfig_Validation_HubCanaryStages(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 100
	cfg.Federation.Hub.Canary.Stages = []CanaryStageConfig{
		{Percentage: 150, WaitSeconds: 60, AutoRollback: true},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "percentage")
}

func TestFederationConfig_Validation_HubCanaryStages_NotSorted(t *testing.T) {
	cfg := defaultValidConfig()
	cfg.Agent.Mode = "hub"
	cfg.Federation.Enabled = true
	cfg.Federation.Hub.ListenAddr = ":9443"
	cfg.Federation.Hub.Region = "us-east"
	cfg.Federation.Hub.MaxLeaves = 100
	cfg.Federation.Hub.Canary.Stages = []CanaryStageConfig{
		{Percentage: 50, WaitSeconds: 60},
		{Percentage: 10, WaitSeconds: 60},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sorted")
}

// defaultValidConfig returns a minimal valid standalone config.
func defaultValidConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			ID:                     "test-agent",
			Name:                   "test",
			IntervalSeconds:        10,
			ShutdownTimeoutSeconds: 30,
		},
		Server:     ServerConfig{ListenAddr: "127.0.0.1:18080"},
		Executor:   ExecutorConfig{TimeoutSeconds: 10, AllowedCommands: []string{"echo"}, MaxOutputBytes: 65536},
		Reporter:   ReporterConfig{Mode: "stdout", TimeoutSeconds: 5, RetryCount: 3, RetryIntervalMS: 500},
		Auth:       AuthConfig{Enabled: false},
		Prometheus: PrometheusConfig{Enabled: true, Path: "/metrics"},
		GRPC:       GRPCConfig{ServerAddr: "localhost:443", HeartbeatIntervalSeconds: 15, ReconnectInitialBackoffMS: 1000, ReconnectMaxBackoffMS: 30000},
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/config/ -run TestFederationConfig -v 2>&1 | head -20`
Expected: FAIL — `FederationConfig` type does not exist

- [ ] **Step 3: Add FederationConfig types to config.go**

Add these types after the existing `WASMConfig` struct (around line 280):

```go
// Agent mode constants.
const (
	AgentModeStandalone = "standalone"
	AgentModeHub        = "hub"
	AgentModeLeaf       = "leaf"
)

// FederationConfig controls the multi-cluster federation subsystem.
type FederationConfig struct {
	Enabled bool                 `mapstructure:"enabled"`
	Hub     FederationHubConfig  `mapstructure:"hub"`
	Leaf    FederationLeafConfig `mapstructure:"leaf"`
}

// FederationHubConfig controls Hub mode behavior.
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

// FederationLeafConfig controls Leaf mode behavior.
type FederationLeafConfig struct {
	HubAddr                 string                 `mapstructure:"hub_addr"`
	ReconnectIntervalSec    int                    `mapstructure:"reconnect_interval_seconds"`
	ReportIntervalSec       int                    `mapstructure:"report_interval_seconds"`
	Fallback                FallbackConfig         `mapstructure:"fallback"`
}

// FederationSecurity holds security config for Hub-Leaf communication.
type FederationSecurity struct {
	MTLS MTLSConfig `mapstructure:"mtls"`
	PSK  string     `mapstructure:"psk"`
}

// GroupRuleConfig defines a single group matching rule.
type GroupRuleConfig struct {
	Name  string            `mapstructure:"name"`
	Match map[string]string `mapstructure:"match"`
}

// ConfigLevelsConfig holds multi-level configuration for Hub.
type ConfigLevelsConfig struct {
	Global  map[string]interface{}            `mapstructure:"global"`
	Regions map[string]map[string]interface{} `mapstructure:"regions"`
	Groups  map[string]map[string]interface{} `mapstructure:"groups"`
	Agents  map[string]map[string]interface{} `mapstructure:"agents"`
}

// CanaryConfig defines canary rollout policy.
type CanaryConfig struct {
	Strategy string            `mapstructure:"strategy"`
	Stages   []CanaryStageConfig `mapstructure:"stages"`
}

// CanaryStageConfig defines a single canary stage.
type CanaryStageConfig struct {
	Percentage    int  `mapstructure:"percentage"`
	WaitSeconds   int  `mapstructure:"wait_seconds"`
	AutoRollback  bool `mapstructure:"auto_rollback"`
}

// OperationsConfig controls batch operation behavior.
type OperationsConfig struct {
	MaxConcurrent            int                `mapstructure:"max_concurrent"`
	DefaultTimeoutSeconds    int                `mapstructure:"default_timeout_seconds"`
	RetryPolicy              RetryPolicyConfig  `mapstructure:"retry_policy"`
}

// RetryPolicyConfig defines retry behavior for operations.
type RetryPolicyConfig struct {
	MaxRetries      int `mapstructure:"max_retries"`
	BackoffSeconds  int `mapstructure:"backoff_seconds"`
}

// FallbackConfig controls Leaf fallback behavior.
type FallbackConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	Mode               string `mapstructure:"mode"`
	PlatformAddr       string `mapstructure:"platform_addr"`
	CheckIntervalSec   int    `mapstructure:"check_interval_seconds"`
}
```

- [ ] **Step 4: Add Federation field to Config struct**

Modify the `Config` struct (around line 11) to add the Federation field:

```go
// Config is the root runtime configuration.
type Config struct {
	Agent         AgentConfig         `mapstructure:"agent"`
	Server        ServerConfig        `mapstructure:"server"`
	Executor      ExecutorConfig      `mapstructure:"executor"`
	Reporter      ReporterConfig      `mapstructure:"reporter"`
	Auth          AuthConfig          `mapstructure:"auth"`
	Prometheus    PrometheusConfig    `mapstructure:"prometheus"`
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
```

- [ ] **Step 5: Add Mode field to AgentConfig**

Modify `AgentConfig` (around line 33):

```go
// AgentConfig controls agent identity and collection cadence.
type AgentConfig struct {
	ID                     string          `mapstructure:"id"`
	Name                   string          `mapstructure:"name"`
	Mode                   string          `mapstructure:"mode"` // standalone | hub | leaf
	IntervalSeconds        int             `mapstructure:"interval_seconds"`
	ShutdownTimeoutSeconds int             `mapstructure:"shutdown_timeout_seconds"`
	AuditLog               AuditLogConfig  `mapstructure:"audit_log"`
}
```

- [ ] **Step 6: Add federation validation to Validate()**

Add at the end of the `Validate()` method, before the final `return nil` (around line 590):

```go
	// Federation validation (only when enabled and mode is hub or leaf).
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
			// Validate canary stages are sorted by percentage.
			for i, stage := range c.Federation.Hub.Canary.Stages {
				if stage.Percentage <= 0 || stage.Percentage > 100 {
					return fmt.Errorf("federation.hub.canary.stages[%d].percentage must be 1-100", i)
				}
				if i > 0 && stage.Percentage <= c.Federation.Hub.Canary.Stages[i-1].Percentage {
					return fmt.Errorf("federation.hub.canary.stages must be sorted by percentage ascending")
				}
			}
		case AgentModeLeaf:
			if strings.TrimSpace(c.Federation.Leaf.HubAddr) == "" {
				return fmt.Errorf("federation.leaf.hub_addr is required when agent.mode=leaf")
			}
		}
	}
```

- [ ] **Step 7: Add federation defaults to Load()**

Add after the existing defaults (around line 350):

```go
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
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/config/ -run TestFederationConfig -v`
Expected: PASS

- [ ] **Step 9: Run all config tests to ensure no regression**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/config/config.go internal/config/federation_test.go
git commit -m "feat(federation): add federation config types and validation"
```

---

### Task 2: Proto Definitions and Code Generation

**Files:**
- Create: `proto/federation.proto`
- Modify: `proto/agent.proto` (add FederationReport to PlatformMessage)

- [ ] **Step 1: Create federation.proto**

```protobuf
// proto/federation.proto
syntax = "proto3";
package opsagent.federation;
option go_package = "github.com/cy77cc/opsagent/internal/federation/proto";

// FederationService runs on the Hub and accepts connections from Leaf agents.
service FederationService {
  rpc Register(FedAgentRegistration) returns (FedRegisterResponse);
  rpc Heartbeat(FedHeartbeatRequest) returns (FedHeartbeatResponse);
  rpc ReportMetrics(FedMetricReport) returns (FedMetricAck);
  rpc StreamConfigUpdates(FedConfigStreamRequest) returns (stream FedConfigUpdate);
  rpc StreamCommands(FedCommandStreamRequest) returns (stream FedCommandRequest);
  rpc ReportCommandResult(FedCommandResult) returns (FedCommandResultAck);
  rpc ReportHealth(FedHealthReport) returns (FedHealthAck);
}

enum AgentMode {
  MODE_STANDALONE = 0;
  MODE_HUB = 1;
  MODE_LEAF = 2;
}

message FedAgentRegistration {
  string agent_id = 1;
  string hostname = 2;
  string ip = 3;
  string version = 4;
  map<string, string> labels = 10;
  map<string, string> auto_labels = 11;
  repeated string capabilities = 12;
  AgentMode mode = 13;
  string region = 14;
}

message FedRegisterResponse {
  bool accepted = 1;
  string assigned_region = 2;
  repeated string assigned_groups = 3;
  string config_version = 4;
  bytes initial_config = 5;
  string rejection_reason = 6;
}

message FedHeartbeatRequest {
  string agent_id = 1;
  int64 timestamp = 2;
  map<string, string> labels = 3;
}

message FedHeartbeatResponse {
  bool ok = 1;
  string config_version = 2;
  bool config_update_available = 3;
}

message FedMetricReport {
  string agent_id = 1;
  repeated FedMetric metrics = 2;
  int64 timestamp = 3;
}

message FedMetric {
  string name = 1;
  map<string, string> tags = 2;
  repeated FedField fields = 3;
  int64 timestamp_ms = 4;
}

message FedField {
  string key = 1;
  oneof value {
    double double_value = 2;
    int64 int_value = 3;
    string string_value = 4;
    bool bool_value = 5;
  }
}

message FedMetricAck {
  bool accepted = 1;
  int64 received_count = 2;
}

message FedConfigStreamRequest {
  string agent_id = 1;
  string current_version = 2;
}

message FedConfigUpdate {
  string config_version = 1;
  bytes config_yaml = 2;
  int64 timestamp = 3;
  string source = 4;
}

message FedCommandStreamRequest {
  string agent_id = 1;
}

message FedCommandRequest {
  string command_id = 1;
  string type = 2;
  bytes payload = 3;
  int32 timeout_seconds = 4;
}

message FedCommandResult {
  string command_id = 1;
  string agent_id = 2;
  int32 exit_code = 3;
  bytes stdout = 4;
  bytes stderr = 5;
  int64 duration_ms = 6;
  bool timed_out = 7;
}

message FedCommandResultAck {
  bool accepted = 1;
}

message FedHealthReport {
  string agent_id = 1;
  string status = 2;
  map<string, string> subsystem_status = 3;
  int64 timestamp = 4;
}

message FedHealthAck {
  bool ok = 1;
}

// Hub → Platform messages
message FederationReport {
  string hub_id = 1;
  string region = 2;
  repeated LeafInfo leaves = 3;
  repeated GroupInfo groups = 4;
  repeated OperationStatus operations = 5;
}

message LeafInfo {
  string agent_id = 1;
  map<string, string> labels = 2;
  repeated string groups = 3;
  string status = 4;
  int64 last_seen = 5;
  string config_version = 6;
}

message GroupInfo {
  string name = 1;
  int32 member_count = 2;
  int32 online_count = 3;
  map<string, string> match_criteria = 4;
}

message OperationStatus {
  string operation_id = 1;
  string type = 2;
  string target_group = 3;
  string status = 4;
  int32 total = 5;
  int32 success = 6;
  int32 failed = 7;
  int32 pending = 8;
}
```

- [ ] **Step 2: Add FederationReport to agent.proto PlatformMessage**

In `proto/agent.proto`, add to the `PlatformMessage` oneof (after `AgentUpdate`):

```protobuf
message PlatformMessage {
  oneof payload {
    ExecuteCommand exec_command = 1;
    ExecuteScript exec_script = 2;
    CancelJob cancel_job = 3;
    ConfigUpdate config_update = 4;
    Ack ack = 5;
    HealthCheckRequest health_check = 6;
    // Gateway tunnel
    TunnelData tunnel_data = 7;
    TunnelClose tunnel_close = 8;
    ProxyCommandRequest proxy_command = 9;
    AgentUpdate agent_update = 10;
    FederationReport federation_report = 11;
  }
}
```

- [ ] **Step 3: Generate Go code from proto**

Run: `cd /Users/zhangdp/project/opsagent && make proto`
Expected: Generated files in `internal/federation/proto/` and updated `internal/grpcclient/proto/`

- [ ] **Step 4: Verify generated code compiles**

Run: `cd /Users/zhangdp/project/opsagent && go build ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add proto/federation.proto proto/agent.proto internal/federation/proto/ internal/grpcclient/proto/
git commit -m "feat(federation): add federation proto definitions and generate Go code"
```

---

### Task 3: Label Collection

**Files:**
- Create: `internal/federation/labels.go`
- Create: `internal/federation/labels_test.go`

- [ ] **Step 1: Write failing tests for CollectAutoLabels**

```go
// internal/federation/labels_test.go
package federation

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectAutoLabels_ReturnsOSAndArch(t *testing.T) {
	labels := CollectAutoLabels()

	require.Contains(t, labels, "os")
	require.Contains(t, labels, "arch")
	assert.Equal(t, runtime.GOOS, labels["os"])
	assert.Equal(t, runtime.GOARCH, labels["arch"])
}

func TestCollectAutoLabels_ReturnsHostname(t *testing.T) {
	labels := CollectAutoLabels()

	require.Contains(t, labels, "hostname")
	assert.NotEmpty(t, labels["hostname"])
}

func TestCollectAutoLabels_ReturnsKernelVersion(t *testing.T) {
	labels := CollectAutoLabels()

	// kernel_version may be empty on non-Linux, that's ok
	if runtime.GOOS == "linux" {
		assert.Contains(t, labels, "kernel_version")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestCollectAutoLabels -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement CollectAutoLabels**

```go
// internal/federation/labels.go
package federation

import (
	"os"
	"runtime"
)

// CollectAutoLabels gathers system metadata labels automatically.
// These labels are reported during Leaf registration and heartbeat.
func CollectAutoLabels() map[string]string {
	hostname, _ := os.Hostname()

	labels := map[string]string{
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
		"hostname": hostname,
	}

	if kernel := getKernelVersion(); kernel != "" {
		labels["kernel_version"] = kernel
	}

	return labels
}

// getKernelVersion reads the kernel version from /proc/version on Linux.
func getKernelVersion() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return ""
	}
	// /proc/version format: "Linux version X.Y.Z ..."
	// Return first line trimmed
	s := string(data)
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestCollectAutoLabels -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/labels.go internal/federation/labels_test.go
git commit -m "feat(federation): add auto-label collection for Leaf agents"
```

---

### Task 4: Leaf State Management

**Files:**
- Create: `internal/federation/leaf_state.go`

- [ ] **Step 1: Write failing tests for LeafState**

```go
// internal/federation/leaf_state_test.go
package federation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLeafState_IsOnline_WhenRecentlySeen(t *testing.T) {
	ls := &LeafState{
		AgentID:  "agent-001",
		LastSeen: time.Now(),
	}
	assert.True(t, ls.IsOnline(60*time.Second))
}

func TestLeafState_IsOnline_WhenStale(t *testing.T) {
	ls := &LeafState{
		AgentID:  "agent-001",
		LastSeen: time.Now().Add(-120 * time.Second),
	}
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
	assert.Equal(t, "web", all["role"])
	assert.Equal(t, "linux", all["os"])
	assert.Equal(t, "amd64", all["arch"])
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestLeafState -v`
Expected: FAIL — `LeafState` not defined

- [ ] **Step 3: Implement LeafState**

```go
// internal/federation/leaf_state.go
package federation

import "time"

const (
	LeafStatusOnline   = "online"
	LeafStatusOffline  = "offline"
	LeafStatusDegraded = "degraded"
)

// LeafState holds the runtime state of a connected Leaf agent.
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

// IsOnline returns true if the Leaf was seen within the given timeout.
func (ls *LeafState) IsOnline(timeout time.Duration) bool {
	return time.Since(ls.LastSeen) <= timeout
}

// AllLabels returns the merged label set (manual labels override auto labels).
func (ls *LeafState) AllLabels() map[string]string {
	result := make(map[string]string, len(ls.AutoLabels)+len(ls.Labels))
	for k, v := range ls.AutoLabels {
		result[k] = v
	}
	for k, v := range ls.Labels {
		result[k] = v
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestLeafState -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/leaf_state.go internal/federation/leaf_state_test.go
git commit -m "feat(federation): add LeafState struct for tracking connected Leaves"
```

---

### Task 5: Group Engine

**Files:**
- Create: `internal/federation/group.go`
- Create: `internal/federation/group_test.go`

- [ ] **Step 1: Write failing tests for GroupEngine**

```go
// internal/federation/group_test.go
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

	leaf := &LeafState{
		AgentID: "agent-001",
		Labels:  map[string]string{"env": "prod", "role": "web"},
	}
	groups := ge.Evaluate(leaf)
	assert.Equal(t, []string{"prod-web"}, groups)
}

func TestGroupEngine_Evaluate_NoMatch(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod-web", Match: map[string]string{"env": "prod", "role": "web"}},
	})

	leaf := &LeafState{
		AgentID: "agent-001",
		Labels:  map[string]string{"env": "staging", "role": "db"},
	}
	groups := ge.Evaluate(leaf)
	assert.Empty(t, groups)
}

func TestGroupEngine_Evaluate_MultipleGroups(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
		{Name: "web", Match: map[string]string{"role": "web"}},
	})

	leaf := &LeafState{
		AgentID: "agent-001",
		Labels:  map[string]string{"env": "prod", "role": "web"},
	}
	groups := ge.Evaluate(leaf)
	assert.Contains(t, groups, "prod")
	assert.Contains(t, groups, "web")
	assert.Len(t, groups, 2)
}

func TestGroupEngine_Evaluate_PartialMatch(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod-web", Match: map[string]string{"env": "prod", "role": "web"}},
	})

	leaf := &LeafState{
		AgentID: "agent-001",
		Labels:  map[string]string{"env": "prod", "role": "db"},
	}
	groups := ge.Evaluate(leaf)
	assert.Empty(t, groups)
}

func TestGroupEngine_UpdateLeaf_UpdatesGroups(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})

	leaf := &LeafState{
		AgentID:  "agent-001",
		Labels:   map[string]string{"env": "prod"},
		LastSeen: time.Now(),
	}
	ge.UpdateLeaf(leaf)

	members := ge.GetGroupMembers("prod")
	assert.Equal(t, []string{"agent-001"}, members)
}

func TestGroupEngine_RemoveLeaf(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})

	leaf := &LeafState{
		AgentID:  "agent-001",
		Labels:   map[string]string{"env": "prod"},
		LastSeen: time.Now(),
	}
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

	leaf := &LeafState{
		AgentID:  "agent-001",
		Labels:   map[string]string{"env": "prod"},
		LastSeen: time.Now(),
	}
	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("prod"), "agent-001")

	// Change labels
	leaf.Labels = map[string]string{"env": "staging"}
	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("staging"), "agent-001")
	assert.NotContains(t, ge.GetGroupMembers("prod"), "agent-001")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestGroupEngine -v`
Expected: FAIL — `GroupEngine` not defined

- [ ] **Step 3: Implement GroupEngine**

```go
// internal/federation/group.go
package federation

import (
	"sort"
	"sync"
)

// GroupRule defines a named group with label matching criteria.
type GroupRule struct {
	Name  string            `json:"name"`
	Match map[string]string `json:"match"`
}

// GroupEngine manages dynamic grouping of Leaf agents based on labels.
type GroupEngine struct {
	mu         sync.RWMutex
	rules      []GroupRule
	leafStates map[string]*LeafState
	groupIndex map[string]map[string]bool // group_name → set of agent_ids
}

// NewGroupEngine creates a new GroupEngine with the given rules.
func NewGroupEngine(rules []GroupRule) *GroupEngine {
	return &GroupEngine{
		rules:      rules,
		leafStates: make(map[string]*LeafState),
		groupIndex: make(map[string]map[string]bool),
	}
}

// Evaluate returns the list of group names the leaf belongs to.
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

// UpdateLeaf registers or updates a leaf and recalculates its groups.
func (ge *GroupEngine) UpdateLeaf(leaf *LeafState) {
	ge.mu.Lock()
	defer ge.mu.Unlock()

	oldGroups := ge.leafGroups(leaf.AgentID)
	newGroups := ge.Evaluate(leaf)

	// Remove from old groups
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

// RemoveLeaf removes a leaf from all groups.
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

// GetGroupMembers returns the agent IDs in the given group.
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

// GetLeaf returns the state of a leaf by agent ID.
func (ge *GroupEngine) GetLeaf(agentID string) *LeafState {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	return ge.leafStates[agentID]
}

// GetAllLeaves returns all tracked leaf states.
func (ge *GroupEngine) GetAllLeaves() map[string]*LeafState {
	ge.mu.RLock()
	defer ge.mu.RUnlock()
	result := make(map[string]*LeafState, len(ge.leafStates))
	for k, v := range ge.leafStates {
		result[k] = v
	}
	return result
}

// leafGroups returns the groups an agent belongs to (caller must hold lock).
func (ge *GroupEngine) leafGroups(agentID string) []string {
	var groups []string
	for g, members := range ge.groupIndex {
		if members[agentID] {
			groups = append(groups, g)
		}
	}
	return groups
}

// matchesAll checks if labels contain all key-value pairs in match.
func matchesAll(labels, match map[string]string) bool {
	for k, v := range match {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// contains checks if a string slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestGroupEngine -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/group.go internal/federation/group_test.go
git commit -m "feat(federation): add GroupEngine for dynamic Leaf grouping"
```

---

### Task 6: Multi-Level Config Distributor

**Files:**
- Create: `internal/federation/config_distributor.go`
- Create: `internal/federation/config_distributor_test.go`

- [ ] **Step 1: Write failing tests for ConfigDistributor**

```go
// internal/federation/config_distributor_test.go
package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDistributor_ResolveConfig_GlobalOnly(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
			},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", nil)
	require.NoError(t, err)
	assert.Equal(t, 30, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_RegionOverridesGlobal(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
			},
		},
		Regions: map[string]map[string]interface{}{
			"us-east": {
				"collector": map[string]interface{}{
					"interval_seconds": 15,
				},
			},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", nil)
	require.NoError(t, err)
	assert.Equal(t, 15, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_GroupOverridesRegion(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
			},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {
				"collector": map[string]interface{}{
					"interval_seconds": 10,
				},
			},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)
	assert.Equal(t, 10, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_AgentOverridesGroup(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
			},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {
				"collector": map[string]interface{}{
					"interval_seconds": 10,
				},
			},
		},
		Agents: map[string]map[string]interface{}{
			"agent-001": {
				"collector": map[string]interface{}{
					"interval_seconds": 5,
				},
			},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)
	assert.Equal(t, 5, cfg["collector"].(map[string]interface{})["interval_seconds"])
}

func TestConfigDistributor_ResolveConfig_ListReplacesNotMerges(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"inputs": []interface{}{"cpu", "memory"},
			},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {
				"collector": map[string]interface{}{
					"inputs": []interface{}{"cpu", "memory", "net"},
				},
			},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)
	inputs := cfg["collector"].(map[string]interface{})["inputs"].([]interface{})
	assert.Equal(t, []interface{}{"cpu", "memory", "net"}, inputs)
}

func TestConfigDistributor_GetConfigVersion_SameConfigSameVersion(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"key": "value",
		},
	}, NewGroupEngine(nil))

	v1, _ := cd.GetConfigVersion("agent-001", "us-east", nil)
	v2, _ := cd.GetConfigVersion("agent-001", "us-east", nil)
	assert.Equal(t, v1, v2)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestConfigDistributor -v`
Expected: FAIL — `ConfigDistributor` not defined

- [ ] **Step 3: Implement ConfigDistributor**

```go
// internal/federation/config_distributor.go
package federation

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// ConfigLevels holds the multi-level configuration hierarchy.
type ConfigLevels struct {
	Global  map[string]interface{}            `json:"global"`
	Regions map[string]map[string]interface{} `json:"regions"`
	Groups  map[string]map[string]interface{} `json:"groups"`
	Agents  map[string]map[string]interface{} `json:"agents"`
}

// ConfigDistributor resolves and distributes multi-level configuration.
type ConfigDistributor struct {
	levels ConfigLevels
	engine *GroupEngine
}

// NewConfigDistributor creates a new ConfigDistributor.
func NewConfigDistributor(levels ConfigLevels, engine *GroupEngine) *ConfigDistributor {
	return &ConfigDistributor{
		levels: levels,
		engine: engine,
	}
}

// ResolveConfig merges config from global → region → group → agent levels.
// Scalars: child overrides parent. Lists: child replaces parent entirely.
func (cd *ConfigDistributor) ResolveConfig(agentID, region string, groups []string) (map[string]interface{}, error) {
	result := deepCopyMap(cd.levels.Global)

	// Apply region config
	if regionCfg, ok := cd.levels.Regions[region]; ok {
		result = deepMerge(result, regionCfg)
	}

	// Apply group configs (in alphabetical order for determinism)
	if len(groups) > 0 {
		sortedGroups := make([]string, len(groups))
		copy(sortedGroups, groups)
		// Simple sort for determinism
		for i := 0; i < len(sortedGroups); i++ {
			for j := i + 1; j < len(sortedGroups); j++ {
				if sortedGroups[i] > sortedGroups[j] {
					sortedGroups[i], sortedGroups[j] = sortedGroups[j], sortedGroups[i]
				}
			}
		}
		for _, g := range sortedGroups {
			if groupCfg, ok := cd.levels.Groups[g]; ok {
				result = deepMerge(result, groupCfg)
			}
		}
	}

	// Apply agent config
	if agentCfg, ok := cd.levels.Agents[agentID]; ok {
		result = deepMerge(result, agentCfg)
	}

	return result, nil
}

// GetConfigVersion returns a deterministic hash of the resolved config.
func (cd *ConfigDistributor) GetConfigVersion(agentID, region string, groups []string) (string, error) {
	cfg, err := cd.ResolveConfig(agentID, region, groups)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:8]), nil
}

// UpdateLevels replaces the config levels.
func (cd *ConfigDistributor) UpdateLevels(levels ConfigLevels) {
	cd.levels = levels
}

// deepMerge merges src into dst. Scalars: src overrides dst.
// Lists and slices: src replaces dst entirely.
func deepMerge(dst, src map[string]interface{}) map[string]interface{} {
	result := deepCopyMap(dst)
	for k, srcVal := range src {
		dstVal, exists := result[k]
		if !exists {
			result[k] = deepCopy(srcVal)
			continue
		}

		srcMap, srcIsMap := srcVal.(map[string]interface{})
		dstMap, dstIsMap := dstVal.(map[string]interface{})

		if srcIsMap && dstIsMap {
			result[k] = deepMerge(dstMap, srcMap)
		} else {
			// Scalar or list: src replaces dst
			result[k] = deepCopy(srcVal)
		}
	}
	return result
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = deepCopy(v)
	}
	return result
}

// deepCopy creates a deep copy of a value.
func deepCopy(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return deepCopyMap(val)
	case []interface{}:
		cp := make([]interface{}, len(val))
		for i, item := range val {
			cp[i] = deepCopy(item)
		}
		return cp
	default:
		return v
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestConfigDistributor -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/config_distributor.go internal/federation/config_distributor_test.go
git commit -m "feat(federation): add multi-level config distributor with inheritance"
```

---

### Task 7: Canary Strategy

**Files:**
- Create: `internal/federation/canary.go`
- Create: `internal/federation/canary_test.go`

- [ ] **Step 1: Write failing tests for CanaryStrategy**

```go
// internal/federation/canary_test.go
package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanaryStrategy_SelectSubset_PercentageBased(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60, AutoRollback: true},
	})

	agents := make([]string, 100)
	for i := range agents {
		agents[i] = fmt.Sprintf("agent-%03d", i)
	}

	subset, err := cs.SelectSubset(agents, 0)
	require.NoError(t, err)
	assert.Len(t, subset, 10) // 10% of 100
}

func TestCanaryStrategy_SelectSubset_RoundsCorrectly(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60},
	})

	agents := make([]string, 15)
	for i := range agents {
		agents[i] = fmt.Sprintf("agent-%03d", i)
	}

	subset, err := cs.SelectSubset(agents, 0)
	require.NoError(t, err)
	assert.Len(t, subset, 2) // 10% of 15 = 1.5, rounds to 2
}

func TestCanaryStrategy_SelectSubset_SecondStage(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60},
		{Percentage: 30, WaitSeconds: 120},
	})

	agents := make([]string, 100)
	for i := range agents {
		agents[i] = fmt.Sprintf("agent-%03d", i)
	}

	subset, err := cs.SelectSubset(agents, 1)
	require.NoError(t, err)
	assert.Len(t, subset, 30) // 30% of 100
}

func TestCanaryStrategy_SelectSubset_OutOfRange(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10, WaitSeconds: 60},
	})

	_, err := cs.SelectSubset([]string{"a", "b"}, 1)
	assert.Error(t, err)
}

func TestCanaryStrategy_TotalStages(t *testing.T) {
	cs := NewCanaryStrategy([]CanaryStage{
		{Percentage: 10},
		{Percentage: 30},
		{Percentage: 100},
	})
	assert.Equal(t, 3, cs.TotalStages())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestCanaryStrategy -v`
Expected: FAIL — `CanaryStrategy` not defined

- [ ] **Step 3: Implement CanaryStrategy**

```go
// internal/federation/canary.go
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

// CanaryStrategy manages percentage-based gradual rollout.
type CanaryStrategy struct {
	stages []CanaryStage
}

// NewCanaryStrategy creates a CanaryStrategy with the given stages.
func NewCanaryStrategy(stages []CanaryStage) *CanaryStrategy {
	return &CanaryStrategy{stages: stages}
}

// TotalStages returns the number of canary stages.
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

// SelectSubset selects the subset of agents for the given stage index.
// For stage 0, it selects percentage% of all agents.
// For stage N, it selects (stageN% - stageN-1%) additional agents.
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

	// For subsequent stages, we need the cumulative count minus previous stage count
	if stageIndex > 0 {
		prevCount := int(math.Round(float64(total) * float64(cs.stages[stageIndex-1].Percentage) / 100.0))
		count = count - prevCount
	}

	if count <= 0 {
		count = 1 // At least 1 agent per stage
	}
	if count > total {
		count = total
	}

	// Deterministic selection: take the first N agents from the sorted list
	// (agents are assumed to be pre-shuffled by the caller if randomness is desired)
	return agents[:count], nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestCanaryStrategy -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/canary.go internal/federation/canary_test.go
git commit -m "feat(federation): add canary strategy for gradual rollout"
```

---

### Task 8: Prometheus Metrics

**Files:**
- Create: `internal/federation/metrics.go`

- [ ] **Step 1: Write failing test for metrics registration**

```go
// internal/federation/metrics_test.go
package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterMetrics_DoesNotPanic(t *testing.T) {
	// Should not panic on first call
	assert.NotPanics(t, func() {
		RegisterMetrics()
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestRegisterMetrics -v`
Expected: FAIL — `RegisterMetrics` not defined

- [ ] **Step 3: Implement metrics registration**

```go
// internal/federation/metrics.go
package federation

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	LeavesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opsagent_federation_leaves_total",
			Help: "Number of connected Leaf agents by status",
		},
		[]string{"region", "status"},
	)

	GroupsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opsagent_federation_groups_total",
			Help: "Number of configured groups",
		},
		[]string{"region"},
	)

	OperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opsagent_federation_operations_total",
			Help: "Total batch operations by type and status",
		},
		[]string{"type", "status"},
	)

	OperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opsagent_federation_operation_duration_seconds",
			Help:    "Duration of batch operations",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	MetricsReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opsagent_federation_metrics_received_total",
			Help: "Total metrics received from Leaf agents",
		},
		[]string{"region"},
	)

	MetricsForwarded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opsagent_federation_metrics_forwarded_total",
			Help: "Total metrics forwarded to platform",
		},
		[]string{"region"},
	)

	ConfigDistributionDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "opsagent_federation_config_distribution_duration_seconds",
			Help:    "Duration of config distribution to Leaf agents",
			Buckets: prometheus.DefBuckets,
		},
	)

	HubConnected = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opsagent_leaf_hub_connected",
			Help: "Whether the Leaf is connected to Hub (1) or not (0)",
		},
		[]string{"hub_addr"},
	)

	FallbackActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "opsagent_leaf_fallback_active",
			Help: "Whether the Leaf is in fallback mode (1) or not (0)",
		},
	)
)

// RegisterMetrics registers all federation Prometheus metrics.
func RegisterMetrics() {
	prometheus.MustRegister(
		LeavesTotal,
		GroupsTotal,
		OperationsTotal,
		OperationDuration,
		MetricsReceived,
		MetricsForwarded,
		ConfigDistributionDuration,
		HubConnected,
		FallbackActive,
	)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestRegisterMetrics -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/metrics.go internal/federation/metrics_test.go
git commit -m "feat(federation): add Prometheus metrics for federation subsystem"
```

---

### Task 9: Federation Interfaces

**Files:**
- Modify: `internal/app/interfaces.go`

- [ ] **Step 1: Add FederationHub and FederationLeaf interfaces**

Add after the existing `Gateway` interface (around line 86):

```go
// FederationHub manages the Hub mode — aggregating Leaf agents, distributing
// config, and coordinating batch operations.
type FederationHub interface {
	Start(ctx context.Context) error
	Stop() error
	HealthStatus() health.Status
}

// FederationLeaf manages the Leaf mode — connecting to Hub, reporting metrics,
// and receiving config updates.
type FederationLeaf interface {
	Start(ctx context.Context) error
	Stop() error
	HealthStatus() health.Status
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /Users/zhangdp/project/opsagent && go build ./...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/app/interfaces.go
git commit -m "feat(federation): add FederationHub and FederationLeaf interfaces"
```

---

### Task 10: Hub gRPC Server

**Files:**
- Create: `internal/federation/server.go`

- [ ] **Step 1: Write failing tests for HubServer registration**

```go
// internal/federation/server_test.go
package federation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubServer_Register_AcceptsValidLeaf(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{"key": "value"},
	}, ge)

	srv := NewHubServer(HubServerConfig{
		Region:          "us-east",
		MaxLeaves:       100,
		GroupEngine:     ge,
		ConfigDistributor: cd,
	})

	resp, err := srv.Register(context.Background(), &FedAgentRegistration{
		AgentId:  "agent-001",
		Hostname: "web-01",
		Ip:       "10.0.1.1",
		Labels:   map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	assert.True(t, resp.Accepted)
	assert.Contains(t, resp.AssignedGroups, "prod")
	assert.NotEmpty(t, resp.ConfigVersion)
}

func TestHubServer_Register_RejectsWhenFull(t *testing.T) {
	ge := NewGroupEngine(nil)
	cd := NewConfigDistributor(ConfigLevels{}, ge)

	srv := NewHubServer(HubServerConfig{
		Region:    "us-east",
		MaxLeaves: 1,
		GroupEngine: ge,
		ConfigDistributor: cd,
	})

	// First registration succeeds
	_, err := srv.Register(context.Background(), &FedAgentRegistration{
		AgentId: "agent-001",
	})
	require.NoError(t, err)

	// Second registration rejected
	resp, err := srv.Register(context.Background(), &FedAgentRegistration{
		AgentId: "agent-002",
	})
	require.NoError(t, err)
	assert.False(t, resp.Accepted)
	assert.Contains(t, resp.RejectionReason, "full")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestHubServer -v`
Expected: FAIL — `HubServer` not defined

- [ ] **Step 3: Implement HubServer**

```go
// internal/federation/server.go
package federation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// HubServerConfig holds configuration for the Hub gRPC server.
type HubServerConfig struct {
	Region            string
	MaxLeaves         int
	ListenAddr        string
	HeartbeatTimeout  time.Duration
	GroupEngine       *GroupEngine
	ConfigDistributor *ConfigDistributor
	Logger            zerolog.Logger
}

// HubServer implements the FederationService gRPC server.
type HubServer struct {
	cfg          HubServerConfig
	mu           sync.RWMutex
	leaves       map[string]*LeafState
	configPushCh map[string]chan *FedConfigUpdate
}

// NewHubServer creates a new HubServer.
func NewHubServer(cfg HubServerConfig) *HubServer {
	return &HubServer{
		cfg:          cfg,
		leaves:       make(map[string]*LeafState),
		configPushCh: make(map[string]chan *FedConfigUpdate),
	}
}

// Register handles a Leaf agent registration request.
func (s *HubServer) Register(ctx context.Context, req *FedAgentRegistration) (*FedRegisterResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check capacity
	if len(s.leaves) >= s.cfg.MaxLeaves {
		return &FedRegisterResponse{
			Accepted:        false,
			RejectionReason: fmt.Sprintf("hub capacity full (%d/%d)", len(s.leaves), s.cfg.MaxLeaves),
		}, nil
	}

	leaf := &LeafState{
		AgentID:    req.AgentId,
		Hostname:   req.Hostname,
		IP:         req.Ip,
		Version:    req.Version,
		Labels:     req.Labels,
		AutoLabels: req.AutoLabels,
		LastSeen:   time.Now(),
		Status:     LeafStatusOnline,
	}

	// Evaluate groups
	groups := s.cfg.GroupEngine.Evaluate(leaf)
	leaf.Groups = groups

	// Store leaf state
	s.leaves[req.AgentId] = leaf
	s.cfg.GroupEngine.UpdateLeaf(leaf)

	// Resolve initial config
	configVersion, _ := s.cfg.ConfigDistributor.GetConfigVersion(req.AgentId, s.cfg.Region, groups)
	leaf.ConfigVersion = configVersion

	// Create config push channel
	s.configPushCh[req.AgentId] = make(chan *FedConfigUpdate, 10)

	s.cfg.Logger.Info().
		Str("agent_id", req.AgentId).
		Strs("groups", groups).
		Msg("Leaf registered")

	return &FedRegisterResponse{
		Accepted:       true,
		AssignedRegion: s.cfg.Region,
		AssignedGroups: groups,
		ConfigVersion:  configVersion,
	}, nil
}

// Heartbeat handles a Leaf heartbeat request.
func (s *HubServer) Heartbeat(ctx context.Context, req *FedHeartbeatRequest) (*FedHeartbeatResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	leaf, ok := s.leaves[req.AgentId]
	if !ok {
		return &FedHeartbeatResponse{Ok: false}, nil
	}

	leaf.LastSeen = time.Now()
	leaf.Status = LeafStatusOnline

	// Update labels if provided
	if len(req.Labels) > 0 {
		leaf.Labels = req.Labels
		s.cfg.GroupEngine.UpdateLeaf(leaf)
	}

	// Check if config update is available
	configVersion, _ := s.cfg.ConfigDistributor.GetConfigVersion(req.AgentId, s.cfg.Region, leaf.Groups)
	updateAvailable := configVersion != leaf.ConfigVersion

	return &FedHeartbeatResponse{
		Ok:                  true,
		ConfigVersion:       configVersion,
		ConfigUpdateAvailable: updateAvailable,
	}, nil
}

// GetLeaves returns a snapshot of all connected leaves.
func (s *HubServer) GetLeaves() map[string]*LeafState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*LeafState, len(s.leaves))
	for k, v := range s.leaves {
		result[k] = v
	}
	return result
}
```

- [ ] **Step 4: Create stub types for FedAgentRegistration etc.**

Since proto generation may not be done yet, create stub types that will be replaced:

```go
// internal/federation/types.go
package federation

// Stub types until proto generation produces the real ones.
// These will be replaced by generated protobuf types.

type FedAgentRegistration struct {
	AgentId    string            `json:"agent_id"`
	Hostname   string            `json:"hostname"`
	Ip         string            `json:"ip"`
	Version    string            `json:"version"`
	Labels     map[string]string `json:"labels"`
	AutoLabels map[string]string `json:"auto_labels"`
	Capabilities []string        `json:"capabilities"`
	Mode       int32             `json:"mode"`
	Region     string            `json:"region"`
}

type FedRegisterResponse struct {
	Accepted        bool     `json:"accepted"`
	AssignedRegion  string   `json:"assigned_region"`
	AssignedGroups  []string `json:"assigned_groups"`
	ConfigVersion   string   `json:"config_version"`
	InitialConfig   []byte   `json:"initial_config"`
	RejectionReason string   `json:"rejection_reason"`
}

type FedHeartbeatRequest struct {
	AgentId   string            `json:"agent_id"`
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
}

type FedHeartbeatResponse struct {
	Ok                    bool   `json:"ok"`
	ConfigVersion         string `json:"config_version"`
	ConfigUpdateAvailable bool   `json:"config_update_available"`
}

type FedMetricReport struct {
	AgentId   string      `json:"agent_id"`
	Metrics   []*FedMetric `json:"metrics"`
	Timestamp int64       `json:"timestamp"`
}

type FedMetric struct {
	Name      string            `json:"name"`
	Tags      map[string]string `json:"tags"`
	Fields    []*FedField       `json:"fields"`
	TimestampMs int64           `json:"timestamp_ms"`
}

type FedField struct {
	Key         string  `json:"key"`
	DoubleValue float64 `json:"double_value,omitempty"`
	IntValue    int64   `json:"int_value,omitempty"`
	StringValue string  `json:"string_value,omitempty"`
	BoolValue   bool    `json:"bool_value,omitempty"`
}

type FedMetricAck struct {
	Accepted      bool  `json:"accepted"`
	ReceivedCount int64 `json:"received_count"`
}

type FedConfigStreamRequest struct {
	AgentId        string `json:"agent_id"`
	CurrentVersion string `json:"current_version"`
}

type FedConfigUpdate struct {
	ConfigVersion string `json:"config_version"`
	ConfigYaml    []byte `json:"config_yaml"`
	Timestamp     int64  `json:"timestamp"`
	Source        string `json:"source"`
}

type FedCommandStreamRequest struct {
	AgentId string `json:"agent_id"`
}

type FedCommandRequest struct {
	CommandId      string `json:"command_id"`
	Type           string `json:"type"`
	Payload        []byte `json:"payload"`
	TimeoutSeconds int32  `json:"timeout_seconds"`
}

type FedCommandResult struct {
	CommandId   string `json:"command_id"`
	AgentId     string `json:"agent_id"`
	ExitCode    int32  `json:"exit_code"`
	Stdout      []byte `json:"stdout"`
	Stderr      []byte `json:"stderr"`
	DurationMs  int64  `json:"duration_ms"`
	TimedOut    bool   `json:"timed_out"`
}

type FedCommandResultAck struct {
	Accepted bool `json:"accepted"`
}

type FedHealthReport struct {
	AgentId          string            `json:"agent_id"`
	Status           string            `json:"status"`
	SubsystemStatus  map[string]string `json:"subsystem_status"`
	Timestamp        int64             `json:"timestamp"`
}

type FedHealthAck struct {
	Ok bool `json:"ok"`
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestHubServer -v`
Expected: PASS

- [ ] **Step 6: Run all federation tests**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/federation/server.go internal/federation/types.go internal/federation/server_test.go
git commit -m "feat(federation): add HubServer for Leaf registration and heartbeat"
```

---

### Task 11: Leaf gRPC Client

**Files:**
- Create: `internal/federation/client.go`

- [ ] **Step 1: Write failing tests for LeafClient**

```go
// internal/federation/client_test.go
package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLeafClient_NewLeafClient(t *testing.T) {
	lc := NewLeafClient(LeafClientConfig{
		AgentID:         "agent-001",
		HubAddr:         "hub.example.com:9443",
		ReconnectSec:    5,
		ReportIntervalSec: 30,
	})
	assert.NotNil(t, lc)
	assert.Equal(t, "agent-001", lc.agentID)
	assert.False(t, lc.IsConnected())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestLeafClient -v`
Expected: FAIL — `LeafClient` not defined

- [ ] **Step 3: Implement LeafClient**

```go
// internal/federation/client.go
package federation

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// LeafClientConfig holds configuration for the Leaf gRPC client.
type LeafClientConfig struct {
	AgentID           string
	HubAddr           string
	Labels            map[string]string
	AutoLabels        map[string]string
	ReconnectSec      int
	ReportIntervalSec int
	Logger            zerolog.Logger
}

// LeafClient connects to a Hub and reports metrics/health.
type LeafClient struct {
	cfg       LeafClientConfig
	mu        sync.RWMutex
	connected bool
	cancel    context.CancelFunc
}

// NewLeafClient creates a new LeafClient.
func NewLeafClient(cfg LeafClientConfig) *LeafClient {
	return &LeafClient{
		cfg: cfg,
	}
}

// Start begins the connection to the Hub.
func (lc *LeafClient) Start(ctx context.Context) error {
	ctx, lc.cancel = context.WithCancel(ctx)

	go lc.connectLoop(ctx)
	return nil
}

// Stop disconnects from the Hub.
func (lc *LeafClient) Stop() {
	if lc.cancel != nil {
		lc.cancel()
	}
	lc.mu.Lock()
	lc.connected = false
	lc.mu.Unlock()
}

// IsConnected returns whether the Leaf is connected to the Hub.
func (lc *LeafClient) IsConnected() bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.connected
}

// HealthStatus returns the health status of the Leaf client.
func (lc *LeafClient) HealthStatus() map[string]interface{} {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return map[string]interface{}{
		"connected": lc.connected,
		"hub_addr":  lc.cfg.HubAddr,
	}
}

// connectLoop attempts to connect to the Hub with reconnection logic.
func (lc *LeafClient) connectLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := lc.connect(ctx)
		if err != nil {
			lc.cfg.Logger.Warn().Err(err).Msg("Hub connection failed, retrying")
			lc.mu.Lock()
			lc.connected = false
			lc.mu.Unlock()

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(lc.cfg.ReconnectSec) * time.Second):
			}
		}
	}
}

// connect establishes a single connection to the Hub.
func (lc *LeafClient) connect(ctx context.Context) error {
	// TODO: implement actual gRPC connection in Task 13 (Hub orchestrator)
	// For now, this is a placeholder that sets connected state
	lc.mu.Lock()
	lc.connected = true
	lc.mu.Unlock()

	lc.cfg.Logger.Info().Str("hub_addr", lc.cfg.HubAddr).Msg("Connected to Hub")

	// Block until context cancelled
	<-ctx.Done()
	return ctx.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestLeafClient -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/client.go internal/federation/client_test.go
git commit -m "feat(federation): add LeafClient for Hub connection"
```

---

### Task 12: Leaf Fallback Logic

**Files:**
- Create: `internal/federation/fallback.go`

- [ ] **Step 1: Write failing tests for FallbackManager**

```go
// internal/federation/fallback_test.go
package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFallbackManager_New(t *testing.T) {
	fm := NewFallbackManager(FallbackConfig{
		Enabled:          true,
		Mode:             "standalone",
		PlatformAddr:     "platform:443",
		CheckIntervalSec: 30,
	})
	assert.NotNil(t, fm)
	assert.False(t, fm.IsActive())
}

func TestFallbackManager_Activate(t *testing.T) {
	fm := NewFallbackManager(FallbackConfig{
		Enabled: true,
		Mode:    "standalone",
	})
	fm.Activate()
	assert.True(t, fm.IsActive())
}

func TestFallbackManager_Deactivate(t *testing.T) {
	fm := NewFallbackManager(FallbackConfig{
		Enabled: true,
		Mode:    "standalone",
	})
	fm.Activate()
	assert.True(t, fm.IsActive())
	fm.Deactivate()
	assert.False(t, fm.IsActive())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestFallbackManager -v`
Expected: FAIL — `FallbackManager` not defined

- [ ] **Step 3: Implement FallbackManager**

```go
// internal/federation/fallback.go
package federation

import "sync"

// FallbackManager handles Leaf fallback to standalone mode when Hub is unavailable.
type FallbackManager struct {
	cfg      FallbackConfig
	mu       sync.RWMutex
	active   bool
}

// NewFallbackManager creates a new FallbackManager.
func NewFallbackManager(cfg FallbackConfig) *FallbackManager {
	return &FallbackManager{
		cfg: cfg,
	}
}

// IsActive returns whether fallback mode is currently active.
func (fm *FallbackManager) IsActive() bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.active
}

// Activate switches to fallback mode.
func (fm *FallbackManager) Activate() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.active = true
}

// Deactivate switches back to normal federation mode.
func (fm *FallbackManager) Deactivate() {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	fm.active = false
}

// Config returns the fallback configuration.
func (fm *FallbackManager) Config() FallbackConfig {
	return fm.cfg
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestFallbackManager -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/fallback.go internal/federation/fallback_test.go
git commit -m "feat(federation): add FallbackManager for Leaf degradation"
```

---

### Task 13: Hub Orchestrator

**Files:**
- Create: `internal/federation/hub.go`

- [ ] **Step 1: Write failing tests for Hub**

```go
// internal/federation/hub_test.go
package federation

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHub_New(t *testing.T) {
	hub := NewHub(HubConfig{
		ListenAddr:   ":9443",
		Region:       "us-east",
		MaxLeaves:    100,
		Logger:       zerolog.Nop(),
	})
	assert.NotNil(t, hub)
}

func TestHub_HealthStatus(t *testing.T) {
	hub := NewHub(HubConfig{
		ListenAddr: ":9443",
		Region:     "us-east",
		MaxLeaves:  100,
		Logger:     zerolog.Nop(),
	})

	status := hub.HealthStatus()
	assert.Equal(t, "stopped", status["status"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestHub -v`
Expected: FAIL — `Hub` not defined

- [ ] **Step 3: Implement Hub orchestrator**

```go
// internal/federation/hub.go
package federation

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

// HubConfig holds configuration for the Hub mode.
type HubConfig struct {
	ListenAddr    string
	Region        string
	MaxLeaves     int
	Groups        []GroupRule
	ConfigLevels  ConfigLevels
	Logger        zerolog.Logger
}

// Hub orchestrates all Hub-mode subsystems.
type Hub struct {
	cfg               HubConfig
	mu                sync.RWMutex
	server            *HubServer
	groupEngine       *GroupEngine
	configDistributor *ConfigDistributor
	running           bool
	cancel            context.CancelFunc
}

// NewHub creates a new Hub orchestrator.
func NewHub(cfg HubConfig) *Hub {
	ge := NewGroupEngine(cfg.Groups)
	cd := NewConfigDistributor(cfg.ConfigLevels, ge)

	srv := NewHubServer(HubServerConfig{
		Region:            cfg.Region,
		MaxLeaves:         cfg.MaxLeaves,
		ListenAddr:        cfg.ListenAddr,
		GroupEngine:       ge,
		ConfigDistributor: cd,
		Logger:            cfg.Logger,
	})

	return &Hub{
		cfg:               cfg,
		server:            srv,
		groupEngine:       ge,
		configDistributor: cd,
	}
}

// Start begins the Hub subsystems.
func (h *Hub) Start(ctx context.Context) error {
	ctx, h.cancel = context.WithCancel(ctx)

	h.mu.Lock()
	h.running = true
	h.mu.Unlock()

	h.cfg.Logger.Info().
		Str("region", h.cfg.Region).
		Str("listen_addr", h.cfg.ListenAddr).
		Int("max_leaves", h.cfg.MaxLeaves).
		Msg("Hub started")

	// Block until context cancelled
	<-ctx.Done()
	return nil
}

// Stop shuts down the Hub subsystems.
func (h *Hub) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()
	h.cfg.Logger.Info().Msg("Hub stopped")
}

// HealthStatus returns the Hub health status.
func (h *Hub) HealthStatus() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := "stopped"
	if h.running {
		status = "running"
	}

	leaves := h.server.GetLeaves()
	onlineCount := 0
	for _, l := range leaves {
		if l.Status == LeafStatusOnline {
			onlineCount++
		}
	}

	return map[string]interface{}{
		"status":         status,
		"region":         h.cfg.Region,
		"leaves_total":   len(leaves),
		"leaves_online":  onlineCount,
		"leaves_offline": len(leaves) - onlineCount,
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestHub -v`
Expected: PASS

- [ ] **Step 5: Run all federation tests**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/federation/hub.go internal/federation/hub_test.go
git commit -m "feat(federation): add Hub orchestrator for Hub mode"
```

---

### Task 14: Operation Manager

**Files:**
- Create: `internal/federation/operation.go`

- [ ] **Step 1: Write failing tests for OperationManager**

```go
// internal/federation/operation_test.go
package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationManager_Create(t *testing.T) {
	om := NewOperationManager()
	op, err := om.Create("config_update", "prod-web", map[string]string{"key": "value"})
	require.NoError(t, err)
	assert.NotEmpty(t, op.ID)
	assert.Equal(t, "config_update", op.Type)
	assert.Equal(t, "prod-web", op.TargetGroup)
	assert.Equal(t, "pending", op.Status)
}

func TestOperationManager_GetStatus(t *testing.T) {
	om := NewOperationManager()
	op, _ := om.Create("restart", "prod-web", nil)

	status, err := om.GetStatus(op.ID)
	require.NoError(t, err)
	assert.Equal(t, op.ID, status.ID)
	assert.Equal(t, "pending", status.Status)
}

func TestOperationManager_GetStatus_NotFound(t *testing.T) {
	om := NewOperationManager()
	_, err := om.GetStatus("nonexistent")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestOperationManager -v`
Expected: FAIL — `OperationManager` not defined

- [ ] **Step 3: Implement OperationManager**

```go
// internal/federation/operation.go
package federation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

const (
	OpStatusPending         = "pending"
	OpStatusRunning         = "running"
	OpStatusCompleted       = "completed"
	OpStatusPartialFailure  = "partial_failure"
	OpStatusFailed          = "failed"
	OpStatusRolledBack      = "rolled_back"
)

// Operation represents a batch operation targeting a group of Leaf agents.
type Operation struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	TargetGroup string                 `json:"target_group"`
	Params      map[string]string      `json:"params"`
	Status      string                 `json:"status"`
	LeafResults map[string]*LeafOpResult `json:"leaf_results"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// LeafOpResult tracks the result of an operation on a single Leaf.
type LeafOpResult struct {
	Status     string    `json:"status"` // pending, dispatched, applying, success, failed, rolling_back, rolled_back
	Error      string    `json:"error,omitempty"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

// OperationStatus is the public status view of an operation.
type OperationStatus struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	TargetGroup string `json:"target_group"`
	Status      string `json:"status"`
	Total       int    `json:"total"`
	Success     int    `json:"success"`
	Failed      int    `json:"failed"`
	Pending     int    `json:"pending"`
}

// OperationManager manages batch operations.
type OperationManager struct {
	mu         sync.RWMutex
	operations map[string]*Operation
}

// NewOperationManager creates a new OperationManager.
func NewOperationManager() *OperationManager {
	return &OperationManager{
		operations: make(map[string]*Operation),
	}
}

// Create creates a new batch operation.
func (om *OperationManager) Create(opType, targetGroup string, params map[string]string) (*Operation, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}

	op := &Operation{
		ID:          id,
		Type:        opType,
		TargetGroup: targetGroup,
		Params:      params,
		Status:      OpStatusPending,
		LeafResults: make(map[string]*LeafOpResult),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	om.mu.Lock()
	om.operations[id] = op
	om.mu.Unlock()

	return op, nil
}

// GetStatus returns the status of an operation.
func (om *OperationManager) GetStatus(opID string) (*OperationStatus, error) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	op, ok := om.operations[opID]
	if !ok {
		return nil, fmt.Errorf("operation %s not found", opID)
	}

	status := &OperationStatus{
		ID:          op.ID,
		Type:        op.Type,
		TargetGroup: op.TargetGroup,
		Status:      op.Status,
	}

	for _, lr := range op.LeafResults {
		status.Total++
		switch lr.Status {
		case "success":
			status.Success++
		case "failed":
			status.Failed++
		default:
			status.Pending++
		}
	}

	return status, nil
}

// generateID generates a random hex ID.
func generateID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "op-" + hex.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestOperationManager -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/federation/operation.go internal/federation/operation_test.go
git commit -m "feat(federation): add OperationManager for batch operations"
```

---

### Task 15: Agent Lifecycle Integration

**Files:**
- Modify: `internal/app/agent.go`

- [ ] **Step 1: Read current agent.go to understand the pattern**

Read `internal/app/agent.go` to understand how subsystems are wired into the Agent lifecycle.

- [ ] **Step 2: Add federation fields to Agent struct**

In `agent.go`, add to the `Agent` struct:

```go
	// Federation
	federationHub  FederationHub
	federationLeaf FederationLeaf
```

- [ ] **Step 3: Wire federation into Agent startup**

In the `New()` function or equivalent constructor, add federation initialization based on `agent.mode`:

```go
	// Initialize federation based on agent mode
	if cfg.Federation.Enabled {
		switch cfg.Agent.Mode {
		case config.AgentModeHub:
			hub := federation.NewHub(federation.HubConfig{
				ListenAddr:   cfg.Federation.Hub.ListenAddr,
				Region:       cfg.Federation.Hub.Region,
				MaxLeaves:    cfg.Federation.Hub.MaxLeaves,
				Groups:       convertGroupRules(cfg.Federation.Hub.Groups),
				ConfigLevels: convertConfigLevels(cfg.Federation.Hub.ConfigLevels),
				Logger:       logger.With().Str("component", "federation-hub").Logger(),
			})
			a.federationHub = hub
			a.subsystems = append(a.subsystems, hub)

		case config.AgentModeLeaf:
			leaf := federation.NewLeafClient(federation.LeafClientConfig{
				AgentID:           cfg.Agent.ID,
				HubAddr:           cfg.Federation.Leaf.HubAddr,
				Labels:            cfg.Agent.Labels,
				AutoLabels:        federation.CollectAutoLabels(),
				ReconnectSec:      cfg.Federation.Leaf.ReconnectIntervalSec,
				ReportIntervalSec: cfg.Federation.Leaf.ReportIntervalSec,
				Logger:            logger.With().Str("component", "federation-leaf").Logger(),
			})
			a.federationLeaf = leaf
			a.subsystems = append(a.subsystems, leaf)
		}
	}
```

- [ ] **Step 4: Add helper conversion functions**

```go
func convertGroupRules(rules []config.GroupRuleConfig) []federation.GroupRule {
	result := make([]federation.GroupRule, len(rules))
	for i, r := range rules {
		result[i] = federation.GroupRule{
			Name:  r.Name,
			Match: r.Match,
		}
	}
	return result
}

func convertConfigLevels(levels config.ConfigLevelsConfig) federation.ConfigLevels {
	return federation.ConfigLevels{
		Global:  levels.Global,
		Regions: levels.Regions,
		Groups:  levels.Groups,
		Agents:  levels.Agents,
	}
}
```

- [ ] **Step 5: Verify compilation**

Run: `cd /Users/zhangdp/project/opsagent && go build ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/agent.go
git commit -m "feat(federation): wire federation Hub/Leaf into Agent lifecycle"
```

---

### Task 16: Config File Update

**Files:**
- Modify: `configs/config.yaml`

- [ ] **Step 1: Add federation section to config.yaml**

Append to the end of `configs/config.yaml`:

```yaml
# Federation (multi-cluster management)
federation:
  enabled: false

  # Hub mode configuration
  hub:
    listen_addr: ":9443"
    region: "us-east"
    max_leaves: 500
    leaf_heartbeat_timeout_seconds: 60
    metrics_aggregation_interval_seconds: 30

    security:
      mtls:
        enabled: false
        cert_file: ""
        key_file: ""
        ca_file: ""
      psk: ""

    groups: []
    #  - name: "prod-web"
    #    match:
    #      env: "production"
    #      role: "web-server"

    config_levels:
      global: {}
      regions: {}
      groups: {}
      agents: {}

    canary:
      strategy: "percentage"
      stages:
        - percentage: 10
          wait_seconds: 60
          auto_rollback: true
        - percentage: 100

    operations:
      max_concurrent: 50
      default_timeout_seconds: 300
      retry_policy:
        max_retries: 3
        backoff_seconds: 10

  # Leaf mode configuration
  leaf:
    hub_addr: ""
    reconnect_interval_seconds: 5
    report_interval_seconds: 30
    fallback:
      enabled: false
      mode: "standalone"
      platform_addr: ""
      check_interval_seconds: 30
```

- [ ] **Step 2: Verify config loads without error**

Run: `cd /Users/zhangdp/project/opsagent && go run ./cmd/agent validate --config ./configs/config.yaml`
Expected: PASS (federation is disabled, so no validation errors)

- [ ] **Step 3: Commit**

```bash
git add configs/config.yaml
git commit -m "feat(federation): add federation config section to default config"
```

---

### Task 17: Integration Tests

**Files:**
- Create: `internal/federation/integration_test.go`

- [ ] **Step 1: Write integration test for Hub-Leaf registration flow**

```go
// internal/federation/integration_test.go
package federation

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_HubLeafRegistration(t *testing.T) {
	// Setup Hub
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
	})
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
			},
		},
	}, ge)

	hub := NewHub(HubConfig{
		ListenAddr:    ":0", // random port for testing
		Region:        "us-east",
		MaxLeaves:     10,
		Groups:        []GroupRule{{Name: "prod", Match: map[string]string{"env": "prod"}}},
		ConfigLevels:  ConfigLevels{Global: map[string]interface{}{"key": "value"}},
		Logger:        zerolog.Nop(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Start(ctx)
	time.Sleep(100 * time.Millisecond) // Let hub start

	// Simulate Leaf registration
	resp, err := hub.server.Register(ctx, &FedAgentRegistration{
		AgentId:  "agent-001",
		Hostname: "web-01",
		Ip:       "10.0.1.1",
		Labels:   map[string]string{"env": "prod"},
	})
	require.NoError(t, err)
	assert.True(t, resp.Accepted)
	assert.Contains(t, resp.AssignedGroups, "prod")
	assert.NotEmpty(t, resp.ConfigVersion)

	// Verify Leaf is tracked
	leaves := hub.server.GetLeaves()
	assert.Len(t, leaves, 1)
	assert.Equal(t, "online", leaves["agent-001"].Status)

	// Simulate heartbeat
	hbResp, err := hub.server.Heartbeat(ctx, &FedHeartbeatRequest{
		AgentId:   "agent-001",
		Timestamp: time.Now().Unix(),
	})
	require.NoError(t, err)
	assert.True(t, hbResp.Ok)

	// Verify health status
	health := hub.HealthStatus()
	assert.Equal(t, "running", health["status"])
	assert.Equal(t, 1, health["leaves_total"])
	assert.Equal(t, 1, health["leaves_online"])
}

func TestIntegration_GroupDynamicUpdate(t *testing.T) {
	ge := NewGroupEngine([]GroupRule{
		{Name: "prod", Match: map[string]string{"env": "prod"}},
		{Name: "staging", Match: map[string]string{"env": "staging"}},
	})

	leaf := &LeafState{
		AgentID:  "agent-001",
		Labels:   map[string]string{"env": "prod"},
		LastSeen: time.Now(),
	}

	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("prod"), "agent-001")
	assert.Empty(t, ge.GetGroupMembers("staging"))

	// Change labels
	leaf.Labels = map[string]string{"env": "staging"}
	ge.UpdateLeaf(leaf)
	assert.Contains(t, ge.GetGroupMembers("staging"), "agent-001")
	assert.NotContains(t, ge.GetGroupMembers("prod"), "agent-001")
}

func TestIntegration_ConfigInheritance(t *testing.T) {
	cd := NewConfigDistributor(ConfigLevels{
		Global: map[string]interface{}{
			"collector": map[string]interface{}{
				"interval_seconds": 30,
				"inputs":           []interface{}{"cpu", "memory"},
			},
		},
		Regions: map[string]map[string]interface{}{
			"us-east": {
				"collector": map[string]interface{}{
					"interval_seconds": 15,
				},
			},
		},
		Groups: map[string]map[string]interface{}{
			"prod-web": {
				"collector": map[string]interface{}{
					"inputs": []interface{}{"cpu", "memory", "net", "http"},
				},
			},
		},
		Agents: map[string]map[string]interface{}{
			"agent-001": {
				"collector": map[string]interface{}{
					"interval_seconds": 5,
				},
			},
		},
	}, NewGroupEngine(nil))

	cfg, err := cd.ResolveConfig("agent-001", "us-east", []string{"prod-web"})
	require.NoError(t, err)

	collector := cfg["collector"].(map[string]interface{})
	assert.Equal(t, 5, collector["interval_seconds"])         // Agent override
	assert.Equal(t, []interface{}{"cpu", "memory", "net", "http"}, collector["inputs"]) // Group override
}
```

- [ ] **Step 2: Run integration tests**

Run: `cd /Users/zhangdp/project/opsagent && go test ./internal/federation/ -run TestIntegration -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/federation/integration_test.go
git commit -m "feat(federation): add integration tests for federation subsystem"
```

---

### Task 18: Final Verification

- [ ] **Step 1: Run all tests**

Run: `cd /Users/zhangdp/project/opsagent && make test-race`
Expected: PASS

- [ ] **Step 2: Run linter**

Run: `cd /Users/zhangdp/project/opsagent && make lint`
Expected: PASS

- [ ] **Step 3: Run vet**

Run: `cd /Users/zhangdp/project/opsagent && make vet`
Expected: PASS

- [ ] **Step 4: Build**

Run: `cd /Users/zhangdp/project/opsagent && make build`
Expected: PASS

- [ ] **Step 5: Commit any fixes**

If any issues were found and fixed:

```bash
git add -A
git commit -m "fix(federation): address lint/vet/test issues"
```
