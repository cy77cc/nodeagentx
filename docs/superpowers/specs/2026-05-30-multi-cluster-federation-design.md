# Multi-Cluster Federation Design

**Date**: 2026-05-30
**Status**: Approved
**Scope**: 多集群/边缘管理 — 大规模 Agent 编组与分发

---

## 1. Overview

### 1.1 Background

OpsAgent 当前以 standalone 模式运行，每个 Agent 直连中心平台 (OpsPilot)。当 Agent 规模扩展到千级、分布跨多个数据中心/区域时，面临以下挑战：

- 中心平台连接数线性增长，成为瓶颈
- 配置变更需要逐台下发，效率低
- 缺乏分组管理能力，无法按角色/环境批量操作
- 边缘节点网络不稳定时缺乏降级策略

### 1.2 Goals

1. **分层 Hub 架构**：引入 Hub Agent 模式，汇聚区域内 Leaf Agent 数据
2. **标签化分组**：Agent 自动上报元数据标签，Hub 动态编组
3. **多级配置继承**：全局→区域→分组→单机四级配置，支持覆盖
4. **批量灰度操作**：按分组批量执行配置变更/升级/重启，支持灰度策略
5. **自动编组**：Agent 自注册时自动采集标签，Hub 自动归组
6. **容错降级**：Hub 不可用时 Leaf 自动降级为 standalone 模式

### 1.3 Non-Goals

- 跨区域服务查询 API（依赖平台侧配合，后续迭代）
- Web UI 管理界面
- Hub 高可用（主备切换，后续迭代）
- 配置变更 diff 可视化

---

## 2. Architecture

### 2.1 Communication Topology

```
            ┌──────────────────────────┐
            │    Center Platform       │
            │    (OpsPilot)            │
            └─────┬──────────┬─────────┘
                  │ gRPC     │ gRPC
            ┌─────┴────┐  ┌─┴────────┐
            │ Hub (A)   │  │ Hub (B)   │
            │ us-east   │  │ eu-west   │
            └──┬──┬──┬──┘  └──┬──┬──┬──┘
               │  │  │        │  │  │
            ┌──┴┐┌┴┐┌┴─┐  ┌──┴┐┌┴┐┌┴─┐
            │ L1││L2││L3│  │ L4││L5││L6│
            └───┘└──┘└──┘  └───┘└──┘└──┘
```

### 2.2 Agent Modes

OpsAgent supports three operating modes. The mode is determined by `agent.mode` in config. When `agent.mode` is unset or `"standalone"`, the agent behaves exactly as before (backward compatible).

| Mode | Role | Connection | Use Case |
|------|------|------------|----------|
| `standalone` | Independent agent, direct to platform | Agent → Platform | Default, backward compatible |
| `leaf` | Regional leaf node, connects to Hub | Leaf → Hub → Platform | Managed business hosts |
| `hub` | Regional aggregation node | Platform ↔ Hub ↔ Leaf | Regional gateway |

**Mode determination logic**:
1. If `agent.mode` is unset or `"standalone"` → standalone mode (no federation)
2. If `agent.mode` is `"hub"` → Hub mode, uses `federation.hub.*` config
3. If `agent.mode` is `"leaf"` → Leaf mode, uses `federation.leaf.*` config

**Key Design Decisions**:

- Hub and Leaf use the same binary, switched via `agent.mode` configuration
- `standalone` mode behavior is completely unchanged, ensuring backward compatibility
- Hub does not collect metrics directly (unless standalone capabilities are also configured), only aggregates and forwards
- Hub has dual roles: `GRPCClient` (to Platform) and `HubServer` (to Leaf agents)
- `federation.enabled` is a separate toggle that must be `true` for hub/leaf modes to activate; if `false` regardless of `agent.mode`, the agent runs in standalone mode

### 2.3 Module Structure

```
internal/federation/
├── hub.go                    # Hub mode main logic
├── server.go                 # gRPC Server implementation (Hub → Leaf)
├── client.go                 # Leaf gRPC Client (Leaf → Hub)
├── group.go                  # Grouping engine
├── config_distributor.go     # Multi-level config inheritance & distribution
├── operation.go              # Batch operation manager
├── canary.go                 # Canary/gradual rollout strategy
├── labels.go                 # Auto label collection
├── leaf_state.go             # Leaf state management
├── metrics.go                # Prometheus metrics
├── fallback.go               # Leaf fallback logic
├── federation_test.go        # Unit tests
└── integration_test.go       # Integration tests
proto/
└── federation.proto          # Federation proto definitions
```

---

## 3. Label-Based Grouping

### 3.1 Label Sources

Agent labels come from two sources:

**Manual labels** (configured in `agent.labels`):

```yaml
agent:
  labels:
    env: "production"
    region: "us-east"
    role: "web-server"
    team: "platform"
```

**Auto labels** (collected at startup by Leaf agent):

| Label Key | Source | Example |
|-----------|--------|---------|
| `os` | `runtime.GOOS` | `linux` |
| `arch` | `runtime.GOARCH` | `amd64` |
| `kernel_version` | `/proc/version` | `5.15.0` |
| `hostname` | `os.Hostname()` | `web-01` |
| `cloud.provider` | Cloud metadata (EC2 IMDS) | `aws` |
| `cloud.region` | Cloud metadata | `us-east-1` |
| `cloud.instance_type` | Cloud metadata | `t3.medium` |
| `cloud.instance_id` | Cloud metadata | `i-0abc123` |

Auto label collection reuses the existing `discovery/metadata` layer.

```go
// internal/federation/labels.go
func CollectAutoLabels() map[string]string {
    labels := map[string]string{
        "os":           runtime.GOOS,
        "arch":         runtime.GOARCH,
        "kernel_version": getKernelVersion(),
        "hostname":     hostname,
    }
    if meta, err := cloudmetadata.Fetch(); err == nil {
        labels["cloud.provider"] = meta.Provider
        labels["cloud.region"] = meta.Region
        labels["cloud.instance_type"] = meta.InstanceType
        labels["cloud.instance_id"] = meta.InstanceID
    }
    return labels
}
```

### 3.2 Group Rules

Group rules are configured on the Hub side. A Leaf belongs to a group if all its labels match the group's `match` criteria.

```yaml
hub:
  groups:
    - name: "prod-web"
      match:
        env: "production"
        role: "web-server"
    - name: "staging-all"
      match:
        env: "staging"
    - name: "us-east-db"
      match:
        region: "us-east"
        role: "database"
```

**Matching semantics**:
- All match criteria must be satisfied (AND logic)
- A Leaf can belong to multiple groups simultaneously
- Groups are recalculated when a Leaf registers or updates its labels
- Group changes trigger config recomputation and distribution

### 3.3 Group Engine

```go
// internal/federation/group.go
type GroupEngine struct {
    mu         sync.RWMutex
    rules      []GroupRule
    leafStates map[string]*LeafState
    groupIndex map[string][]string   // group_name → []agent_id
}

type GroupRule struct {
    Name  string
    Match map[string]string
}

type LeafState struct {
    AgentID       string
    Labels        map[string]string
    AutoLabels    map[string]string
    Groups        []string
    LastSeen      time.Time
    Status        string              // online | offline | degraded
    ConfigVersion string
}

func (e *GroupEngine) Evaluate(leaf *LeafState) []string
func (e *GroupEngine) GetGroupMembers(groupName string) []string
func (e *GroupEngine) UpdateLeaf(leaf *LeafState)
func (e *GroupEngine) RemoveLeaf(agentID string)
```

---

## 4. Multi-Level Config Inheritance

### 4.1 Inheritance Hierarchy

Config supports four-level inheritance, from coarse to fine:

```
Global Config (global)
  └─ Region Config (region)
      └─ Group Config (group)
          └─ Agent Config (agent)
```

**Inheritance rules**:
- Each level only declares fields that need to be overridden; the rest inherits from the parent
- Same-name fields: child overrides parent
- List types (e.g., `inputs`): child replaces parent entirely (no merge to avoid ambiguity)

### 4.2 Config Levels Structure

```yaml
hub:
  config_levels:
    global:
      collector:
        interval_seconds: 30
        inputs:
          - type: cpu
          - type: memory
      sandbox:
        enabled: true
    regions:
      us-east:
        collector:
          interval_seconds: 15
      eu-west:
        collector:
          interval_seconds: 60
    groups:
      prod-web:
        collector:
          inputs:
            - type: cpu
            - type: memory
            - type: net
            - type: http
              config:
                urls: ["http://localhost:8080/health"]
    agents:
      agent-001:
        collector:
          interval_seconds: 5
```

### 4.3 Config Resolution

For agent `agent-001` (belongs to `prod-web` group, in `us-east` region):

| Field | Value | Source |
|-------|-------|--------|
| `interval_seconds` | 5 | Agent level override |
| `inputs` | cpu, memory, net, http | Group level override |
| `sandbox.enabled` | true | Global level inheritance |

```go
// internal/federation/config_distributor.go
type ConfigDistributor struct {
    levels *ConfigLevels
    hub    *Hub
}

type ConfigLevels struct {
    Global  *Config
    Regions map[string]*Config
    Groups  map[string]*Config
    Agents  map[string]*Config
}

func (d *ConfigDistributor) ResolveConfig(agentID string) (*Config, error)
// Merge order: global → region → group → agent
// Uses deep merge with child-wins semantics for scalars, child-replaces for lists

func (d *ConfigDistributor) GetConfigVersion(agentID string) string
// Returns a hash of the resolved config for consistency checking

func (d *ConfigDistributor) Distribute(groupName string, cfg *Config) error
// Push config update to all members of a group
```

### 4.4 Config Distribution Flow

```
Hub config change
  → Identify affected Leaf set (by group/region/agent)
  → Resolve final config for each Leaf
  → Batch push via gRPC Server Stream
  → Leaf applies config and reports status
  → Hub records config version per Leaf
```

The config distribution message extends the existing `ConfigUpdate` proto:

```protobuf
message FederationConfigUpdate {
  string config_version = 1;
  bytes  config_yaml = 2;
  int64  timestamp = 3;
  string source = 4;              // "global" | "region" | "group" | "agent"
}
```

---

## 5. Batch Canary Operations

### 5.1 Supported Operation Types

| Operation | Description | Canary Support |
|-----------|-------------|----------------|
| `config_update` | Config change | Percentage-based batches |
| `binary_upgrade` | Binary upgrade (reuses Updater) | Percentage-based batches |
| `restart` | Restart Agent | Percentage-based batches |
| `health_check` | Batch health check | Full concurrency |
| `command_exec` | Batch command execution | Percentage-based batches |

### 5.2 Canary Policy

```yaml
hub:
  canary:
    strategy: "percentage"      # percentage | count
    stages:
      - percentage: 10
        wait_seconds: 60
        auto_rollback: true
      - percentage: 30
        wait_seconds: 120
        auto_rollback: true
      - percentage: 100
```

**Canary flow**:

```
1. User initiates batch operation (e.g., upgrade prod-web group)
2. Hub selects Leaf subset per canary strategy:
   ├── Stage 1: 10% of Leaf (random selection)
   │   ├── Dispatch upgrade command
   │   ├── Wait 60s, monitor health
   │   └── On failure → auto-rollback + abort remaining stages
   ├── Stage 2: 30% of Leaf
   │   ├── Dispatch upgrade command
   │   ├── Wait 120s
   │   └── On failure → auto-rollback + abort
   └── Stage 3: remaining 100%
       └── Dispatch upgrade command
3. Hub logs operation audit events, reports to platform
```

### 5.3 Operation State Machine

Each batch operation generates an `OperationID` with the following state machine:

```
pending → running → completed
                  → partial_failure
                  → failed
                  → rolled_back
```

Per-Leaf operation state:

```
pending → dispatched → applying → success
                                → failed → rolling_back → rolled_back
```

### 5.4 Operation Manager

```go
// internal/federation/operation.go
type OperationManager struct {
    operations map[string]*Operation
    hub        *Hub
}

type Operation struct {
    ID          string
    Type        string              // config_update | binary_upgrade | restart | ...
    TargetGroup string
    Status      string              // pending | running | completed | ...
    Canary      *CanaryPolicy
    LeafResults map[string]*LeafOpResult
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

func (m *OperationManager) Create(opType, targetGroup string, params map[string]string) (*Operation, error)
func (m *OperationManager) Execute(ctx context.Context, opID string, canary *CanaryPolicy) error
func (m *OperationManager) Rollback(ctx context.Context, opID string) error
func (m *OperationManager) GetStatus(opID string) (*OperationStatus, error)
```

**Operation status API**:

```
GET /api/v1/operations/{operation_id}

Response:
{
  "id": "op-001",
  "type": "binary_upgrade",
  "target_group": "prod-web",
  "status": "running",
  "progress": {
    "total": 100,
    "success": 30,
    "failed": 0,
    "pending": 70
  },
  "stages": [
    {"percentage": 10, "status": "completed", "duration_ms": 65000},
    {"percentage": 30, "status": "running", "elapsed_ms": 45000}
  ]
}
```

---

## 6. gRPC Protocol Extensions

### 6.1 Federation Service (Hub ↔ Leaf)

```protobuf
syntax = "proto3";
package opsagent.federation;

service FederationService {
  // Leaf registration
  rpc Register(AgentRegistration) returns (RegisterResponse);
  // Leaf heartbeat
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  // Leaf reports metrics
  rpc ReportMetrics(MetricReport) returns (MetricAck);
  // Hub pushes config updates (server streaming)
  rpc StreamConfigUpdates(ConfigStreamRequest) returns (stream FederationConfigUpdate);
  // Hub pushes commands (server streaming)
  rpc StreamCommands(CommandStreamRequest) returns (stream CommandRequest);
  // Leaf reports command execution results
  rpc ReportCommandResult(CommandResult) returns (CommandResultAck);
  // Leaf reports health status
  rpc ReportHealth(HealthReport) returns (HealthAck);
}

message AgentRegistration {
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

enum AgentMode {
  STANDALONE = 0;
  HUB = 1;
  LEAF = 2;
}

message RegisterResponse {
  bool accepted = 1;
  string assigned_region = 2;
  repeated string assigned_groups = 3;
  string config_version = 4;
  bytes initial_config = 5;
  string rejection_reason = 6;
}

message HeartbeatRequest {
  string agent_id = 1;
  int64 timestamp = 2;
  map<string, string> labels = 3;   // Labels can be updated on heartbeat
}

message HeartbeatResponse {
  bool ok = 1;
  string config_version = 2;
  bool config_update_available = 3;
}

message MetricReport {
  string agent_id = 1;
  repeated Metric metrics = 2;   // Reuses existing Metric message from collector.proto
  int64 timestamp = 3;
}

message MetricAck {
  bool accepted = 1;
  int64 received_count = 2;
}

message ConfigStreamRequest {
  string agent_id = 1;
  string current_version = 2;
}

message FederationConfigUpdate {
  string config_version = 1;
  bytes config_yaml = 2;
  int64 timestamp = 3;
  string source = 4;
}

message CommandStreamRequest {
  string agent_id = 1;
}

message CommandRequest {
  string command_id = 1;
  string type = 2;
  bytes payload = 3;
  int32 timeout_seconds = 4;
}

message CommandResult {
  string command_id = 1;
  string agent_id = 2;
  int32 exit_code = 3;
  bytes stdout = 4;
  bytes stderr = 5;
  int64 duration_ms = 6;
  bool timed_out = 7;
}

message CommandResultAck {
  bool accepted = 1;
}

message HealthReport {
  string agent_id = 1;
  string status = 2;     // healthy | degraded | unhealthy
  map<string, string> subsystem_status = 3;
  int64 timestamp = 4;
}

message HealthAck {
  bool ok = 1;
}
```

### 6.2 Hub-Platform Extensions

Extending the existing proto with federation-specific messages:

```protobuf
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

---

## 7. Fault Tolerance

### 7.1 Hub Fault Tolerance

- Hub maintains Leaf connection state; timeout without heartbeat marks Leaf as `offline`
- Hub locally caches the last reported metrics from each Leaf; data is not lost during disconnection
- After Hub crash and restart, Leaf automatically reconnects and re-registers (reuses existing exponential backoff reconnection)

### 7.2 Leaf Fallback

When the Hub is unavailable, Leaf degrades to `standalone` mode and connects directly to the platform:

```yaml
leaf:
  fallback:
    enabled: true
    mode: "standalone"
    platform_addr: "platform.example.com:443"
    check_interval_seconds: 30
```

**Fallback flow**:

```
Leaf connects to Hub → Connection fails / heartbeat timeout
  → Activate fallback mode
  → Connect directly to platform using standalone GRPCClient
  → Periodically check Hub availability (every 30s)
  → When Hub recovers → Reconnect and re-register
```

**Note**: Fallback `mode: "standalone"` means the Leaf temporarily behaves like a standalone agent (direct platform connection), but `agent.mode` remains `"leaf"`. When the Hub recovers, the Leaf automatically switches back to federation mode.

### 7.3 Config Distribution Fault Tolerance

- Hub pushes config and waits for Leaf acknowledgment; timeout marks as `config_mismatch`
- Config version checking: Hub periodically exchanges config version numbers with Leaf
- Rollback: Hub can push a rollback command, Leaf restores to the last known good config

---

## 8. Security

### 8.1 Hub-Leaf Communication

- **mTLS**: Reuses existing gRPC mTLS mechanism
- **PSK Authentication**: Leaf must provide a pre-shared key when connecting to Hub (reuses Gateway PSK pattern)

```yaml
hub:
  security:
    mtls:
      enabled: true
      cert_file: "/etc/opsagent/certs/hub.crt"
      key_file: "/etc/opsagent/certs/hub.key"
      ca_file: "/etc/opsagent/certs/ca.crt"
    psk: "shared-secret-key"
```

### 8.2 Operation Authorization

- Batch operations require platform-side authorization
- Hub verifies operation source before execution
- All federation operations are recorded in audit logs

### 8.3 Audit Events

New audit event types:

| Event Type | Component | Action |
|------------|-----------|--------|
| `federation` | `hub` | `leaf_registered`, `leaf_disconnected`, `leaf_fallback` |
| `federation` | `config` | `config_distributed`, `config_applied`, `config_rollback` |
| `federation` | `operation` | `operation_created`, `operation_started`, `operation_completed`, `operation_failed`, `operation_rolled_back` |

---

## 9. Observability

### 9.1 Prometheus Metrics

**Hub metrics**:

```
opsagent_federation_leaves_total{region, status}          # Leaf count by status
opsagent_federation_groups_total{region}                   # Group count
opsagent_federation_config_version{region, group}          # Config version gauge
opsagent_federation_operations_total{type, status}         # Operation counter
opsagent_federation_operation_duration_seconds{type}       # Operation duration histogram
opsagent_federation_metrics_received_total{region}         # Received metrics counter
opsagent_federation_metrics_forwarded_total{region}        # Forwarded metrics counter
opsagent_federation_config_distribution_duration_seconds   # Config distribution latency
```

**Leaf metrics**:

```
opsagent_leaf_hub_connected{hub_addr}                      # Hub connection status
opsagent_leaf_config_version                               # Current config version
opsagent_leaf_fallback_active                              # Whether in fallback mode
opsagent_leaf_hub_latency_seconds                          # Hub round-trip latency
```

### 9.2 Health Check Extension

Hub `/healthz` response includes federation subsystem:

```json
{
  "status": "healthy",
  "subsystems": {
    "federation": {
      "status": "healthy",
      "leaves_online": 45,
      "leaves_offline": 2,
      "groups": 5,
      "pending_operations": 1,
      "config_version": "abc123"
    }
  }
}
```

---

## 10. Configuration Reference

```yaml
# configs/config.yaml - mode selection (top-level, existing section)
agent:
  mode: "standalone"    # standalone | hub | leaf

# configs/config.yaml - federation section
federation:
  enabled: false        # Must be true for hub/leaf modes to activate

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

    # Group rules
    groups: []
    #  - name: "prod-web"
    #    match:
    #      env: "production"
    #      role: "web-server"

    # Multi-level config
    config_levels:
      global: {}
      regions: {}
      groups: {}
      agents: {}

    # Canary policy
    canary:
      strategy: "percentage"
      stages:
        - percentage: 10
          wait_seconds: 60
          auto_rollback: true
        - percentage: 100

    # Batch operations
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

---

## 11. Implementation Scope

| Module | Scope |
|--------|-------|
| Mode switching | standalone/hub/leaf mode config and startup logic |
| Hub Server | gRPC Server accepting Leaf registration, heartbeat, metrics |
| Leaf Client | gRPC Client connecting to Hub, with standalone fallback |
| Group engine | Label matching, dynamic grouping |
| Config distribution | Multi-level inheritance, Hub → Leaf push |
| Batch operations | Config update, upgrade, restart batch execution |
| Canary strategy | Percentage-based batching, auto-rollback |
| Audit logging | Federation operation audit |
| Prometheus metrics | Federation metrics export |
| Testing | Unit tests + integration tests |

---

## 12. Testing Strategy

### 12.1 Unit Tests

- **Group engine**: Label matching, dynamic grouping, group change notifications
- **Config distributor**: Multi-level inheritance, override, version calculation
- **Canary strategy**: Batch logic, rollback trigger
- **Label collection**: Auto labels, cloud metadata integration

### 12.2 Integration Tests

- Hub-Leaf registration, heartbeat, metrics reporting full flow
- Config distribution and application verification
- Batch operation and canary execution
- Leaf fallback to standalone mode
- Hub restart and Leaf reconnection recovery

### 12.3 End-to-End Tests

- Simulate multiple Leaf scenarios, verify grouping, config inheritance, batch operations
- Simulate Hub crash, verify Leaf fallback and recovery
- Simulate network partition, verify timeout and reconnection
