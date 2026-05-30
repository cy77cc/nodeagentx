# OpsAgent Architecture Document

This document describes the overall architecture of OpsAgent, the responsibilities of each subsystem, data flows, and key design decisions.

## 1. Overall Architecture

OpsAgent is a lightweight daemon that runs on the target host, communicating with the operations platform (OpsPilot) via gRPC. The Agent uses a modular design where subsystems are decoupled through interfaces, supporting independent lifecycle management and hot-reloading.

```
+===========================================================================+
|                            OpsPilot (Platform)                             |
|                     gRPC Server (Bidirectional Stream)                      |
+===========================================================================+
         ^  gRPC bidirectional stream (mTLS)
         |  - Heartbeat / Registration / Metrics / Command Execution / Tunnel Data
         v
+===========================================================================+
|                           OpsAgent (Local Host)                             |
+---------------------------------------------------------------------------+
|                                                                           |
|  +-------------------+      +------------------+      +----------------+  |
|  |  Collector         |      |  gRPC Client     |      |  HTTP Server   |  |
|  |  Pipeline          |      |                  |      |                |  |
|  |  +-----------+     |      |  +------------+  |      | /health        |  |
|  |  | Input     |     |      |  | Heartbeat  |  |      | /metrics       |  |
|  |  | (cpu,memory    |  +--->|  | Reconnect  |  |      | /exec          |  |
|  |  |  disk,net...)  |      |  | Cache      |  |      | /tasks         |  |
|  |  +-----+-----+     |      |  | mTLS       |  |      | /config        |  |
|  |        v           |      |  +------------+  |      +--------+-------+  |
|  |  +-----------+     |      +------------------+               |         |
|  |  | Processor |     |                                         |         |
|  |  | (regex,   |     |      +------------------+               |         |
|  |  |  delta,   |     |      |  Task Dispatcher |<--------------+         |
|  |  |  tagger)  |     |      +--------+---------+                          |
|  |  +-----+-----+     |               |                                    |
|  |        v           |       +-------+-------+                            |
|  |  +-----------+     |       |       |       |                            |
|  |  |Aggregator|     |       v       v       v                            |
|  |  |(avg,sum, |     |  +-------+ +------+ +---------+                     |
|  |  | minmax,  |     |  |Executor| |Sandbox| |Plugin  |                    |
|  |  |percentile|     |  |(whitelist| |(nsjail)| |Runtime |                   |
|  |  +-----+-----+   |  | exec)  | |exec) | |(Rust    |                    |
|  |        v          |  +-------+ +------+ | UDS)    |                    |
|  |  +-----------+    |                     +----+----+                    |
|  |  |  Output   |    |                         |                          |
|  |  |(http,prom,|    |                  +------+-------+                  |
|  |  | promrw)   |    |                  | Plugin       |                  |
|  |  +-----------+    |                  | Gateway      |                  |
|  +-------------------+                  | (custom      |                  |
|                                         |  plugins)    |                  |
|  +-------------------+                  +--------------+                  |
|  |  Checker (5 types)|                                                    |
|  |  kernel           |      +------------------+                          |
|  |  filesystem       |      |  Gateway         |                          |
|  |  network          |      |  +------------+  |                          |
|  |  service          |      |  | Tunnel     |  |                          |
|  |  container        |      |  | Pool       |  |                          |
|  +-------------------+      |  +------------+  |                          |
|                             |  | Proxy      |  |                          |
|  +-------------------+      |  | (SSH)      |  |                          |
|  |  Config Reloader  |      |  +------------+  |                          |
|  |  (watch + apply   |      +------------------+                          |
|  |   + rollback)     |                                                     |
|  +-------------------+      +------------------+                          |
|                             |  Audit Logger    |                          |
|                             |  (JSONL rotation)|                          |
|                             +------------------+                          |
+===========================================================================+
```

### Subsystem Overview

| Subsystem | Location | Responsibility |
|-----------|----------|---------------|
| Collector Pipeline | `internal/collector/` | System metric collection, processing, aggregation, and reporting |
| Sandbox Executor | `internal/sandbox/` | Secure command sandbox execution based on nsjail |
| Executor | `internal/executor/` | Allowlist command executor (non-sandbox mode) |
| gRPC Client | `internal/grpcclient/` | Bidirectional gRPC communication with the platform |
| Plugin Runtime | `internal/pluginruntime/` | Rust process management, UDS IPC |
| Plugin Gateway | `internal/pluginruntime/gateway.go` | Custom plugin lifecycle and routing |
| Gateway | `internal/gateway/` | Tunneling and proxy (jump server functionality) |
| Checker | `internal/checker/` | System health checks (5 major categories) |
| HTTP Server | `internal/server/` | Local HTTP API (health/metrics/tasks) |
| Config Reloader | `internal/config/reload.go` | Configuration hot-reloading and atomic rollback |
| Audit Logger | `internal/app/audit.go` | Structured audit logging |
| Log Collector | `internal/logcollector/` | Log collection, parsing, and OTLP output |
| Tracing | `internal/tracing/` | Distributed tracing (OTLP receive/export, batch processing) |
| Dashboard | `internal/dashboard/` | Local embedded HTML dashboard |
| Alerting | `internal/alerting/` | Alert rule engine and Webhook notifications |
| Discovery | `internal/discovery/` | Service auto-discovery (systemd/proc/container/cloud metadata) |
| Templates | `internal/templates/` | Configuration template engine with variable substitution |
| Updater | `internal/updater/` | Auto-update (A/B binary swap, Ed25519 signature verification) |
| WASM Plugin Runtime | `internal/wasm/` | WebAssembly module loading, compilation, and sandboxed execution |
| Plugin Marketplace | `internal/marketplace/` | Remote plugin registry, search, download, and install management |

## 2. Subsystem Responsibilities

### 2.1 Collector Pipeline (Metric Collection Pipeline)

**Responsibility**: Collects system metrics at configurable intervals, processes and aggregates them, then outputs to targets.

The Collector uses a Telegraf-style four-stage pipeline design:

```
Input -> Processor -> Aggregator -> Output
```

Each stage is managed through a plugin registry (`DefaultRegistry`), using the `init()` self-registration pattern. The pipeline supports runtime hot-reloading: the `Scheduler.Reload()` method stops the current pipeline, rebuilds all plugin instances, and restarts collection.

**Key Interfaces**:

```go
// Input collects raw metrics
type Input interface {
    Init(config map[string]interface{}) error
    Gather(acc Accumulator) error
    SampleConfig() string
}

// Processor transforms metrics after collection
type Processor interface {
    Init(config map[string]interface{}) error
    Apply(metrics ...Metric) []Metric
}

// Aggregator aggregates metrics over a time period
type Aggregator interface {
    Init(config map[string]interface{}) error
    Add(m Metric)
    Push(acc Accumulator)
    Reset()
}

// Output sends metrics to external targets
type Output interface {
    Init(config map[string]interface{}) error
    Write(metrics []Metric) error
}
```

**Registered Plugins**:

- Input: `cpu`, `memory`, `disk`, `net`, `process`, `load`, `diskio`, `temp`, `gpu`, `connections`, `http`, `snmp`, `cloud_metadata`
- Processor: `regex`, `delta`, `tagger`, `logparse`
- Aggregator: `avg`, `sum`, `minmax`, `percentile`
- Output: `http`, `prometheus`, `promrw`, `otlp`

**Key Files**: `internal/collector/scheduler.go`, `internal/collector/registry.go`, `internal/collector/inputs/`, `internal/collector/processors/`, `internal/collector/aggregators/`, `internal/collector/outputs/`

### 2.2 Sandbox Executor (Sandbox Executor)

**Responsibility**: Securely executes user commands and scripts in an nsjail sandbox, providing namespace isolation, resource limits, and security policies.

The Sandbox Executor executes commands in an isolated environment through nsjail (a Linux namespace isolation tool). It supports both command and script modes, with a complete security policy engine:

- Command allowlist/blocklist
- Keyword filtering (injection prevention)
- Shell injection detection
- Script size limits
- Resource limits (memory, CPU, PID count)
- Streaming output reading and truncation
- Network isolation modes
- Audit logging

**Key Interfaces**:

```go
// ExecRequest sandbox execution request
type ExecRequest struct {
    TaskID      string
    Command     string
    Args        []string
    Script      string
    Interpreter string
    Env         map[string]string
    Timeout     time.Duration
    SandboxCfg  *SandboxOverride // Per-request resource limit overrides
}

// ExecResult sandbox execution result
type ExecResult struct {
    TaskID    string
    ExitCode  int
    Duration  time.Duration
    TimedOut  bool
    Truncated bool
    Stdout    []byte
    Stderr    []byte
}
```

**Key Files**: `internal/sandbox/executor.go`, `internal/sandbox/nsjail.go`, `internal/sandbox/policy.go`, `internal/sandbox/audit.go`, `internal/sandbox/output_streamer.go`, `internal/sandbox/network.go`

### 2.3 Executor (Allowlist Executor)

**Responsibility**: Executes commands from the allowlist in non-sandbox mode. Provides basic timeout and output limits.

**Key Interfaces**:

```go
type Request struct {
    Command        string
    Args           []string
    TimeoutSeconds int
}

type Result struct {
    ExitCode   int
    Stdout     string
    Stderr     string
    DurationMS int64
}
```

**Key Files**: `internal/executor/executor.go`, `internal/executor/whitelist.go`

### 2.4 gRPC Client (gRPC Client)

**Responsibility**: Maintains a bidirectional gRPC stream connection with the platform (OpsPilot), handling heartbeats, reconnection, metric caching, and command reception.

The gRPC Client is the core component for Agent-platform communication, implementing all data exchange through a single bidirectional stream:

- **Heartbeat**: Periodically sends heartbeats to keep the connection alive
- **Reconnection**: Exponential backoff reconnection strategy (initial 1s, max 30s)
- **Caching**: Metrics are cached in memory during disconnection and sent in batch after reconnection; supports persistence to disk
- **mTLS**: Supports mutual TLS authentication
- **Reception**: `Receiver` handles commands from the platform (task execution, tunnel data, health checks, etc.)

**Key Interfaces** (`GRPCClient`, defined in `internal/app/interfaces.go`):

```go
type GRPCClient interface {
    Start(ctx context.Context) error
    Stop()
    FlushAndStop(ctx context.Context, persistPath string) error
    SendMetrics(metrics []*collector.Metric)
    SendExecOutput(taskID, streamName string, data []byte)
    SendExecResult(result *grpcclient.ExecResult)
    SendHealthCheckResult(result *pb.HealthCheckResult)
    IsConnected() bool
    HealthStatus() health.Status
    SetOnStateChange(fn func(connected bool))
    // Tunnel operations
    SendTunnelOpen(...) error
    SendTunnelData(...) error
    SendTunnelClose(...) error
    // Proxy operations
    SendProxyRegister(...) error
    SendProxyResponse(...) error
    SendProxyMetrics(...) error
}
```

**Key Files**: `internal/grpcclient/client.go`, `internal/grpcclient/receiver.go`, `internal/grpcclient/cache.go`, `internal/grpcclient/proto/`

### 2.5 Plugin Runtime (Plugin Runtime)

**Responsibility**: Manages the lifecycle of Rust processes, performing IPC communication via Unix Domain Socket (UDS).

The Plugin Runtime manages an independent Rust process, executing tasks via JSON-RPC protocol over UDS. The Rust process is automatically started as a child process of the Agent, with support for startup timeouts, concurrent task limits, and chunked transfer.

**Key Interfaces** (defined in `internal/app/interfaces.go`):

```go
type PluginRuntime interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ExecuteTask(ctx context.Context, req pluginruntime.TaskRequest) (*pluginruntime.TaskResponse, error)
    HealthStatus() health.Status
}
```

**Key Files**: `internal/pluginruntime/runtime.go`, `internal/pluginruntime/client.go`

### 2.6 Plugin Gateway (Plugin Gateway)

**Responsibility**: Manages the discovery, lifecycle, routing, and hot-reloading of custom plugins.

The Plugin Gateway scans the plugin directory, automatically discovers and manages independent plugin processes. Each plugin communicates via UDS, supporting task type routing, health checks, automatic restart, and file-change hot-reloading.

**Key Interfaces** (defined in `internal/app/interfaces.go`):

```go
type PluginGateway interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ExecuteTask(ctx context.Context, req pluginruntime.TaskRequest) (*pluginruntime.TaskResponse, error)
    ListPlugins() []pluginruntime.PluginInfo
    GetPlugin(name string) *pluginruntime.PluginInfo
    ReloadPlugin(name string) error
    EnablePlugin(name string) error
    DisablePlugin(name string) error
    OnPluginLoaded(fn func(name string, taskTypes []string))
    OnPluginUnloaded(fn func(name string, taskTypes []string))
    HealthStatus() health.Status
}
```

**Key Files**: `internal/pluginruntime/gateway.go`

### 2.7 Gateway (Jump Server Gateway)

**Responsibility**: Provides jump server functionality in both tunnel and proxy modes, enabling the platform to access internal network hosts.

The Gateway supports two operating modes:

- **Tunnel Mode**: The platform connects to internal network hosts through the Agent's gRPC tunnel, with the Agent acting as a TCP proxy forwarding data.
- **Proxy Mode**: The Agent connects directly to internal network hosts via SSH to execute commands, with results returned to the platform via gRPC.
- **Auto Mode**: Automatically selects the optimal mode.

The Gateway maintains a tunnel connection pool (`tunnel.Pool`), managing tunnel creation, data transfer, and closure.

**Key Interfaces** (defined in `internal/app/interfaces.go`):

```go
type Gateway interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    HandleTunnelData(tunnelID string, data []byte) error
    HandleTunnelClose(tunnelID, reason string) error
    HandleProxyCommand(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error
    HealthStatus() health.Status
}
```

**Internal Interfaces** (defined in `internal/gateway/gateway.go`):

```go
type TunnelSender interface {
    SendTunnelOpen(tunnelID, agentID, hostname, ip string, capabilities []string) error
    SendTunnelData(tunnelID string, payload []byte) error
    SendTunnelClose(tunnelID, reason string) error
}

type ProxySender interface {
    SendProxyRegister(hostID, hostname, ip string, capabilities []string) error
    SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error
    SendProxyMetrics(hostID string, metrics []byte) error
}
```

**Key Files**: `internal/gateway/gateway.go`, `internal/gateway/tunnel/pool.go`, `internal/gateway/proxy/`

### 2.8 Checker (Health Checker)

**Responsibility**: Executes system health check tasks dispatched by the platform, covering 5 major categories of checks.

The Checker uses a registry pattern, managing all checkers through `DefaultRegistry`. Each checker type implements the `Checker` interface:

```go
type Checker interface {
    Type() string
    Check(ctx context.Context, params map[string]string) (*CheckResult, error)
}
```

The `Executor` receives `HealthCheckRequest` dispatched from the platform, routes each item to the corresponding checker, returns intermediate results in real-time via callbacks, and ultimately returns a summary.

**5 Major Checker Categories**:

| Type | Package | Check Content |
|------|---------|---------------|
| kernel | `checker/kernel` | Kernel parameters, version, modules |
| filesystem | `checker/filesystem` | Disk space, mount points, permissions |
| network | `checker/network` | Network connectivity, ports, DNS |
| service | `checker/service` | Service status, port listening |
| container | `checker/container` | Container runtime status, resources |

**Key Files**: `internal/checker/executor.go`, `internal/checker/registry.go`, `internal/checker/kernel/`, `internal/checker/filesystem/`, `internal/checker/network/`, `internal/checker/service/`, `internal/checker/container/`

### 2.9 HTTP Server (Local HTTP Service)

**Responsibility**: Provides local HTTP API for health checks, Prometheus metrics export, command execution, and task management.

The HTTP Server listens on a local address (default `127.0.0.1:18080`), providing the following endpoints:

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Comprehensive health check (aggregates subsystem statuses) |
| `GET /metrics` | Prometheus format metrics export |
| `POST /exec` | Local command execution (allowlist or sandbox) |
| `POST /tasks` | Task submission (routed through Task Dispatcher) |
| `POST /config` | Configuration hot-update |

Supports Bearer Token authentication and independent authentication control for the Prometheus endpoint.

**Key Interfaces** (defined in `internal/app/interfaces.go`):

```go
type HTTPServer interface {
    Start() error
    Shutdown(ctx context.Context) error
    SetLatestMetric(metric *collector.MetricPayload)
    LatestMetricExists() bool
}
```

**Key Files**: `internal/server/server.go`, `internal/server/handlers.go`

### 2.10 Config Reloader (Configuration Hot-Reloader)

**Responsibility**: Monitors configuration changes and coordinates subsystems for atomic configuration hot-updates and rollback.

The Config Reloader implements a two-phase commit style configuration update:

1. Parse new configuration YAML
2. Compute the ChangeSet
3. Call each subsystem's `CanReload()` to check compatibility in order
4. Call `Apply()` to apply the new configuration
5. If any subsystem fails, call `Rollback()` on already-updated subsystems to revert

Each subsystem that supports hot-reloading implements the `Reloader` interface:

```go
type Reloader interface {
    CanReload(cs *ChangeSet) bool
    Apply(newCfg *Config) error
    Rollback(oldCfg *Config) error
}
```

**Key Files**: `internal/config/reload.go`, `internal/config/reload_test.go`

### 2.11 Audit Logger (Audit Logger)

**Responsibility**: Records structured audit events with log rotation support.

The Audit Logger writes audit logs in JSON Lines format, using the lumberjack library for log rotation. Recorded events include:

- Configuration loading/changes
- gRPC connection/disconnection
- Command execution
- Sandbox execution
- Plugin loading/unloading

```go
type AuditEvent struct {
    Timestamp time.Time              `json:"timestamp"`
    EventType string                 `json:"event_type"`
    Component string                 `json:"component"`
    Action    string                 `json:"action"`
    Status    string                 `json:"status"`
    Details   map[string]interface{} `json:"details,omitempty"`
    Error     string                 `json:"error,omitempty"`
}
```

**Key Files**: `internal/app/audit.go`

### 2.12 Log Collector

**Responsibility**: Collects logs from multiple sources, parses them into structured records, and exports via OTLP protocol.

Log Collector supports three input sources: file tail (tracking log file growth line by line), journald (reading systemd journal), and syslog (listening on UDP/TCP syslog ports). Raw logs are processed by the `logparse` processor for regex matching and field extraction, converting them into structured log records. Structured logs are ultimately exported to log backends (such as OpenTelemetry Collector, Loki) via OTLP gRPC/HTTP protocol.

**Key Files**: `internal/logcollector/`

### 2.13 Tracing

**Responsibility**: Receives, processes, and exports OpenTelemetry tracing data for distributed trace collection.

The Tracing subsystem follows the OpenTelemetry Collector architecture, composed of three components in sequence: OTLP Receiver accepts trace data via both gRPC (port 4317) and HTTP (port 4318); Batch Processor performs batch aggregation of spans, balancing latency and throughput through configurable timeout and batch size; OTLP Exporter sends processed trace data to backends (such as Jaeger, Tempo) via gRPC protocol. The tracing subsystem is disabled by default and runs independently when enabled.

**Key Files**: `internal/tracing/`

### 2.14 Dashboard

**Responsibility**: Provides an embedded HTML dashboard for real-time display of Agent status and log streams.

Dashboard is a lightweight web interface embedded in the Agent binary, using Go's `embed` package to compile frontend resources into the executable, requiring no additional static file deployment. The dashboard implements real-time log push via SSE (Server-Sent Events); browsers can receive the Agent's structured log stream after establishing an SSE connection. Dashboard listens on a local HTTP port, intended only for development debugging and on-site troubleshooting, not exposed to the external network.

**Key Files**: `internal/dashboard/`

### 2.15 Alerting

**Responsibility**: Evaluates metrics against configurable rules in real-time, triggers alerts and notifies external systems via Webhook.

The Alerting engine contains three core components: the rule manager loads and hot-reloads alert rules; the evaluation engine periodically evaluates collected metrics against rule expressions to determine if alert conditions are met; the notifier sends alert events to external systems (such as Slack, PagerDuty, DingTalk) via Webhook. The alert state machine manages the alert lifecycle (inactive → pending → firing → resolved), using sustained evaluation intervals and trigger thresholds to avoid alert flapping.

**Key Files**: `internal/alerting/`

### 2.16 Discovery

**Responsibility**: Multi-layer scanning of services running on the host, automatically discovering and outputting them in a unified `Service` structure.

Discovery uses a layered architecture where each layer is responsible for one discovery source. All layers implement the unified `DiscoveryLayer` interface, orchestrated by `DiscoveryService`. Results are deduplicated by `type:name` key to avoid the same service being reported by multiple layers.

**Discovery Layers**:

| Layer | Implementation | Discovery Method |
|------|------|----------|
| systemd | `SystemdLayer` | Calls `systemctl list-units --type=service --state=running` to get running service names and PIDs |
| proc | `ProcLayer` | Scans all LISTEN-state network connections in `/proc`, aggregates by PID, extracts process name, port, and command line |
| container | `ContainerLayer` | Queries Docker Unix Socket `/containers/json` API to discover running containers, port mappings, and labels |
| metadata | `MetadataLayer` | Requests EC2 metadata service (`169.254.169.254`) to get instance ID, type, region, and private IP |

**Key Interfaces**:

```go
// Service represents a discovered service
type Service struct {
    Name         string            `json:"name"`
    Type         string            `json:"type"`
    PID          int               `json:"pid,omitempty"`
    Ports        []int             `json:"ports,omitempty"`
    Labels       map[string]string `json:"labels,omitempty"`
    Metadata     map[string]any    `json:"metadata,omitempty"`
    DiscoveredAt time.Time         `json:"discovered_at"`
}

// DiscoveryLayer is the interface all discovery layers must implement
type DiscoveryLayer interface {
    Name() string
    Discover(ctx context.Context) ([]Service, error)
}
```

**Workflow**:

1. `DiscoveryService.Run()` performs a full discovery scan immediately on startup
2. Subsequently, re-scans periodically at the configured `interval_seconds` interval
3. Each scan iterates all enabled layers, calling `Discover()` to collect services
4. Results are deduplicated by `type:name` and cached, accessible via `LastResults()` for the latest snapshot
5. A single layer failure does not affect other layers; only an error log is recorded

**Key Files**: `internal/discovery/discovery.go`, `internal/discovery/systemd.go`, `internal/discovery/proc.go`, `internal/discovery/container.go`, `internal/discovery/metadata.go`

### 2.17 Templates

**Responsibility**: Provides predefined monitoring configuration templates that generate Collector input plugin configurations through variable substitution.

The Templates subsystem abstracts monitoring configurations into reusable YAML templates, compiled into the binary via Go's `embed.FS`. Each template defines variables (with descriptions, default values, and types); at runtime, `Loader.Apply()` performs variable substitution to generate the final Collector input configuration.

**Template Structure**:

```yaml
name: "nginx"
description: "Nginx web server monitoring"
version: "1.0.0"
variables:
  stub_status_url:
    description: "Nginx stub_status endpoint URL"
    default: "http://127.0.0.1:80/nginx_status"
    type: "string"
collector:
  inputs:
    - type: http
      config:
        urls: ["{{.stub_status_url}}"]
```

**Key Interfaces**:

```go
// Loader loads and manages templates from the embedded filesystem
type Loader struct { ... }

func NewLoader() (*Loader, error)          // Loads all YAML templates from TemplateFS
func (l *Loader) List() []string           // Lists all loaded template names
func (l *Loader) Get(name string) (*Template, error)  // Gets template by name
func (l *Loader) Apply(tmpl *Template, vars map[string]string) (*ApplyResult, error)  // Renders template
```

**Variable Substitution Mechanism**:

1. On load, all YAML files are parsed and registered in an in-memory map
2. When `Apply()` is called, variables are first filled with template-defined defaults
3. User-provided variables override defaults
4. The configuration tree is recursively traversed, applying Go `text/template` rendering to all string values
5. Non-string types (integers, booleans, lists) are preserved as-is
6. The render result is an `ApplyResult` containing the final input plugin configuration list

**Embedding Mechanism**: Uses `//go:embed templates/*.yaml` directive to compile the template directory into the binary's `TemplateFS` variable, requiring no additional template files at deployment.

**Key Files**: `internal/templates/embed.go`, `internal/templates/loader.go`, `internal/templates/templates/`

### 2.18 Updater

**Responsibility**: Securely downloads, verifies, and applies Agent binary updates with rollback support.

Updater implements an A/B binary swap strategy: downloads the new version to a temporary file, performs dual verification with SHA256 checksum and Ed25519 signature, then replaces the current binary via atomic `rename` operation while retaining the old version as backup. On update failure, a single rollback restores the backup version.

**Update Flow**:

```
  Download new binary → SHA256 verify → Ed25519 signature verify → Write temp file → Atomic swap
                                                                      ↓
                                                          Current binary → backup path
                                                          Temp file     → current path
```

**Key Interfaces**:

```go
// UpdateRequest represents an update request
type UpdateRequest struct {
    Version     string
    DownloadURL string
    SHA256      string
    Signature   []byte
}

// Updater handles downloading, verifying, and applying binary updates
type Updater struct { ... }

func New(currentPath, backupPath, downloadDir string, pub ed25519.PublicKey, logger zerolog.Logger) *Updater
func (u *Updater) Apply(req UpdateRequest) error   // Executes full update flow
func (u *Updater) Rollback() error                  // Restores from backup
```

**Security Mechanisms**:

- **SHA256 Checksum**: After download, hash is computed and compared with expected value to prevent transmission corruption
- **Ed25519 Signature Verification**: Uses pre-configured public key to verify binary signature, ensuring trusted source and no tampering
- **Size Limit**: Download content maximum 500MB, aborts immediately if exceeded
- **Timeout Control**: HTTP download timeout of 5 minutes

**A/B Swap Strategy**:

1. Writes new binary to a temporary file under `download_dir`, sets executable permission
2. `rename` current binary to `backup_path` (atomic operation)
3. `rename` temporary file to `current_path` (atomic operation)
4. If step 3 fails, automatically restores backup via `rename` back to `current_path`
5. `Rollback()` can be triggered manually at runtime to restore backup binary to current location

**Key Files**: `internal/updater/updater.go`

### 2.19 WASM Plugin Runtime

**Responsibility**: Loads, compiles, and executes WebAssembly modules, providing a secure isolated plugin execution environment.

WASM Plugin Runtime is based on [wazero](https://github.com/tetratelabs/wazero) (a pure Go WebAssembly runtime), requiring no CGO or external dependencies to run WASM modules. The runtime manages the full lifecycle of WASM modules: loading binaries and manifest files from disk, compiling to machine code, instantiating modules, and executing.

**Manifest Format**:

```yaml
name: "my-plugin"
version: "1.0.0"
runtime: "wasm"
binary_path: "/etc/opsagent/wasm-plugins/my-plugin.wasm"
task_types:
  - "custom-task"
limits:
  max_memory_pages: 256    # 256 * 64 KiB = 16 MiB
  max_table_size: 1024
  max_cpu_seconds: 30
sandbox:
  enabled: true
  network_access: false
  allowed_paths: []
```

**Security Mechanisms**:

- **Memory Limit**: Limits module memory via `max_memory_pages` (default 16 MiB) to prevent memory exhaustion
- **CPU Limit**: Limits execution time via `max_cpu_seconds` to prevent infinite loops
- **Sandbox Mode**: Supports network access control and file path whitelisting
- **Module Isolation**: Each module runs in an independent wazero instance; crashes don't affect other modules
- **No System Calls**: WASM modules run in a pure user-space sandbox, unable to directly access host system calls

**Key Interfaces**:

```go
// WASMRuntime manages compilation, instantiation, and lifecycle of WASM modules
type WASMRuntime struct { ... }

func NewRuntime(ctx context.Context, cfg RuntimeConfig, logger zerolog.Logger) (*WASMRuntime, error)
func (r *WASMRuntime) LoadModule(ctx context.Context, manifestPath string) (*WASMModule, error)
func (r *WASMRuntime) ListModules() []string
func (r *WASMRuntime) Close(ctx context.Context) error

// WASMModule wraps a compiled WASM module
type WASMModule struct {
    Name     string
    Manifest *Manifest
    // ...
}

func (m *WASMModule) Execute(ctx context.Context, input []byte) ([]byte, error)
func (m *WASMModule) Close(ctx context.Context) error
```

**Module Loading Flow**:

1. Read manifest file (YAML), validate required fields (name, version, binary_path, task_types)
2. Apply defaults (memory 16 MiB, CPU 30 seconds, sandbox enabled)
3. Read WASM binary file
4. Call `wazero.Runtime.CompileModule()` to compile to machine code
5. Call `InstantiateModule()` to create module instance
6. Register to internal module map

**Configuration Example** (`configs/config.yaml`):

```yaml
wasm:
  enabled: false
  plugins_dir: "/etc/opsagent/wasm-plugins"
  max_modules: 10
  cache_dir: "/var/lib/opsagent/wasm-cache"
```

**Key Files**: `internal/wasm/runtime.go`, `internal/wasm/module.go`, `internal/wasm/manifest.go`

### 2.20 Plugin Marketplace

**Responsibility**: Provides remote plugin registry, search, download, and install management.

The Plugin Marketplace subsystem contains two core components: `Registry` is responsible for fetching available plugin lists from a remote index and providing search functionality; `Installer` handles downloading, verifying, and managing local plugin installations.

**Registry**:

Registry fetches a JSON-format plugin index (`RegistryIndex`) from a remote URL. The index contains metadata for all available plugins (name, version, description, author, tags, download URL, and checksum). Supports fuzzy search by name and description; the index is cached in memory after first fetch.

```go
// PluginEntry represents a single plugin in the registry
type PluginEntry struct {
    Name        string   `json:"name"`
    Version     string   `json:"version"`
    Description string   `json:"description"`
    Author      string   `json:"author"`
    Homepage    string   `json:"homepage"`
    Tags        []string `json:"tags"`
    DownloadURL string   `json:"download_url"`
    Checksum    string   `json:"checksum"`
}

type Registry struct { ... }

func NewRegistry(indexURL string, client *http.Client) *Registry
func (r *Registry) Search(query string) ([]PluginEntry, error)
func (r *Registry) Get(name string) (*PluginEntry, error)
```

**Installer**:

Installer handles plugin downloading, verification, and lifecycle management. The installation process includes: creating plugin directory, downloading archive, SHA256 checksum verification, writing binary and metadata files. Supports install, uninstall, and listing installed plugins.

```go
type Installer struct { ... }

func NewInstaller(pluginsDir string, client *http.Client) *Installer
func (i *Installer) Install(entry PluginEntry) error
func (i *Installer) Remove(name string) error
func (i *Installer) List() ([]InstalledPlugin, error)
```

**Installation Flow**:

1. Create `{pluginsDir}/{pluginName}/` directory
2. Download plugin archive via HTTP GET
3. Compute SHA256 hash and compare with index checksum
4. Write plugin binary file (set executable permission)
5. Generate `installed.yaml` metadata file (records name, version, installation time)

**Security Mechanisms**:

- **SHA256 Checksum**: Verify checksum after download to prevent transmission corruption or tampering
- **Directory Isolation**: Each plugin installed in an independent directory to avoid file conflicts
- **Metadata Tracking**: Records installation time for version management and auditing

**Key Files**: `internal/marketplace/registry.go`, `internal/marketplace/installer.go`

## 3. Data Flows

### 3.1 Metric Collection Flow

Metrics are collected from Input plugins, transformed by Processors, aggregated by Aggregators, and ultimately reported to the platform through Output plugins and gRPC.

```
  Scheduler             Input            Processor         Aggregator         Output          gRPC Client
     |                   |                  |                  |                 |                 |
     |--- Gather() ----->|                  |                  |                 |                 |
     |                   |--- Accumulator ->|                  |                 |                 |
     |                   |    (raw metrics) |                  |                 |                 |
     |                   |                  |--- Apply() ---->|                 |                 |
     |                   |                  |  (transformed)   |                 |                 |
     |                   |                  |                  |--- Push() ---->|                 |
     |                   |                  |                  |  (aggregated)   |                 |
     |                   |                  |                  |                 |--- Write() ---->|
     |                   |                  |                  |                 |  (output target)|
     |                   |                  |                  |                 |                 |
     |                   |                  |                  |                 |   (also)        |
     |                   |                  |                  |                 |--- SendMetrics()>|
     |                   |                  |                  |                 |                 |-> Platform
```

**Detailed Steps**:

1. `Scheduler` starts an independent goroutine for each Input, calling `Gather()` at the configured interval
2. `Input.Gather()` writes collected raw metrics to the `Accumulator`
3. `Accumulator` passes metrics to `Processor.Apply()` for transformation (regex matching, delta calculation, tag addition, etc.)
4. Processed metrics are collected by `Aggregator.Add()`, with aggregation results periodically output via `Push()`
5. Aggregated metrics are sent to external targets via `Output.Write()` (HTTP, Prometheus remote write, etc.)
6. Simultaneously, metrics are also reported to the platform via `gRPCClient.SendMetrics()`

### 3.2 Command Execution Flow

The platform dispatches execution commands via gRPC, and the Agent selects sandbox mode or allowlist mode for execution based on configuration.

```
  Platform           gRPC Client        Task Dispatcher      Sandbox/Executor
     |                   |                    |                     |
     |-- ExecRequest --->|                    |                     |
     |                   |-- Dispatch() ----->|                     |
     |                   |                    |-- Route by type --->|
     |                   |                    |                     |
     |                   |                    |                     |
     |                   |                    |   [Sandbox Mode]    |
     |                   |                    |<-- nsjail.Execute --|
     |                   |                    |   (namespace        |
     |                   |                    |    isolation,       |
     |                   |                    |    cgroup limits,   |
     |                   |                    |    policy checks)   |
     |                   |                    |                     |
     |                   |                    |   [Allowlist Mode]  |
     |                   |                    |<-- exec.Command ----|
     |                   |                    |   (allowlist check, |
     |                   |                    |    timeout control) |
     |                   |                    |                     |
     |                   |<-- ExecResult -----|                     |
     |<-- SendResult ----|                    |                     |
     |                   |                    |                     |
     |                   |  (streaming output)|                     |
     |                   |<-- stdout/stderr --|                     |
     |<-- SendOutput ----|                    |                     |
```

**Detailed Steps**:

1. The platform sends an execution request via the gRPC bidirectional stream
2. `gRPCClient.Receiver` receives the request and submits it to the `Task Dispatcher`
3. `Dispatcher` routes to the corresponding executor based on task type
4. The executor runs the command:
   - **Sandbox Mode**: Creates an isolated namespace via nsjail, sets cgroup resource limits, checks security policies, and executes the command
   - **Allowlist Mode**: Validates the command against the allowlist, sets a timeout, and executes directly
5. stdout/stderr are streamed back in real-time via `SendExecOutput()`
6. The final result (exit code, duration, timeout status) is sent back via `SendExecResult()`

### 3.3 Plugin Task Flow

Custom plugin tasks are routed to the corresponding plugin process through the Plugin Gateway.

```
  Platform/gRPC      Task Dispatcher      Plugin Gateway        Plugin Process
     |                    |                    |                     |
     |-- TaskRequest ---->|                    |                     |
     |                    |-- ExecuteTask() -->|                     |
     |                    |                    |-- Route by type --->|
     |                    |                    |   (lookup registered|
     |                    |                    |    plugin)          |
     |                    |                    |                     |
     |                    |                    |-- UDS JSON-RPC ---->|
     |                    |                    |   (Unix Domain Socket|
     |                    |                    |    communication)   |
     |                    |                    |                     |
     |                    |                    |<-- TaskResponse -----|
     |                    |<-- Response --------|                     |
     |<-- Result ---------|                    |                     |
```

**Detailed Steps**:

1. The platform or local HTTP API submits a plugin task request
2. `Task Dispatcher` routes the request to `Plugin Gateway`
3. `Plugin Gateway` looks up the registered plugin based on task type (through the task type mapping registered via `OnPluginLoaded` callback)
4. The plugin process is invoked via UDS (Unix Domain Socket) using JSON-RPC protocol
5. The plugin processes the request and returns the result
6. The result is returned to the caller via gRPC or HTTP

## 4. Design Decisions

### 4.1 Why nsjail for Sandboxing

**Choice**: Use Google's open-source nsjail as the command sandbox execution environment.

**Rationale**:

- **Namespace Isolation**: nsjail natively supports Linux namespaces (PID, mount, network, user, cgroup), providing process-level isolation without a container runtime
- **Mature and Stable**: nsjail is maintained by Google, widely used internally by the Chrome team for code execution isolation, and validated in large-scale production environments
- **No Kernel Modules Required**: Does not depend on seccomp-bpf, AppArmor, or other kernel modules that require additional configuration; works out of the box
- **Fine-Grained Control**: Supports precise resource limits (memory, CPU, PID count, file size), network isolation, and filesystem mount control
- **Lightweight**: Process startup overhead is much lower than container solutions (Docker/runc), suitable for high-frequency command execution scenarios

**Alternative Comparison**:

| Solution | Advantages | Disadvantages |
|----------|-----------|---------------|
| nsjail | Lightweight, flexible, no container runtime needed | Linux only |
| Docker | Rich ecosystem | High startup overhead, requires daemon |
| gVisor | Strong isolation | Kernel compatibility issues, performance overhead |
| firejail | Simple | Limited features, weaker security |
| seccomp | Kernel-level | Complex configuration, difficult debugging |

### 4.2 Why gRPC Bidirectional Streaming

**Choice**: Use a single gRPC bidirectional stream as the sole communication channel between the Agent and the platform.

**Rationale**:

- **Single Connection**: All communication multiplexes over one TCP connection, reducing connection management overhead and firewall configuration complexity
- **Server Push**: The platform can push commands to the Agent at any time (execution requests, tunnel data, health checks) without requiring the Agent to poll
- **Efficient Transport**: gRPC is based on HTTP/2, supporting multiplexing, header compression, and binary serialization (protobuf), more efficient than REST APIs
- **Type Safety**: Protobuf defines strongly-typed interfaces with compile-time checking, avoiding runtime serialization errors
- **Streaming**: Supports real-time streaming of stdout/stderr during command execution without waiting for the command to complete
- **mTLS Support**: gRPC natively supports mTLS, simplifying security authentication configuration

### 4.3 Why Rust for the Plugin Runtime

**Choice**: Write the plugin runtime process in Rust, communicating with the Go main process via UDS.

**Rationale**:

- **Performance**: Rust's zero-cost abstractions and GC-free nature ensure low-latency plugin execution with predictable memory behavior
- **Memory Safety**: Rust's ownership system prevents memory safety issues at compile time, reducing the risk of plugin runtime crashes
- **UDS IPC**: Unix Domain Socket provides efficient inter-process communication, avoiding network protocol stack overhead, suitable for high-frequency task invocations
- **Independent Process**: The plugin runtime runs as an independent process; crashes do not affect the Agent main process, supporting independent upgrades and restarts
- **Sandbox-Friendly**: Rust plugins can inherit the Agent's nsjail configuration, running inside the sandbox for an additional layer of security

### 4.4 Why the Telegraf-Style Pipeline

**Choice**: Adopt a four-stage pipeline design: Input -> Processor -> Aggregator -> Output.

**Rationale**:

- **Proven Pattern**: Telegraf is widely used in production environments (InfluxData ecosystem), and this pipeline pattern has been proven reliable and scalable
- **Composability**: Each stage is independent, allowing free combination of different Input, Processor, Aggregator, and Output plugins
- **Hot-Reloading**: Each stage of the pipeline can be independently re-initialized, supporting runtime configuration changes without restarting the Agent
- **Plugin-Based**: Through the registry pattern (`DefaultRegistry`) and `init()` self-registration, new plugins only need to implement the interface and register to be usable
- **Separation of Concerns**: Each stage has clear responsibility boundaries, facilitating independent testing and maintenance

### 4.5 Other Design Decisions

**Interface Isolation**: All subsystems interact with the Agent core through interfaces (defined in `internal/app/interfaces.go`), supporting mock injection for testing. Interface satisfaction is verified at compile time through type assertions.

**Orchestrator Pattern**: The `Agent` struct serves as the orchestrator, responsible for assembling all subsystems and managing their lifecycles (start, stop, health checks). The `Task Dispatcher` handles task routing, dispatching execution requests to the corresponding executors.

**Configuration Validation**: Strict validation is performed during configuration loading (`Config.Validate()`), including required field checks, value range validation, and conditional dependency verification (e.g., nsjail path must be configured when sandbox is enabled). Configuration errors are detected before startup, avoiding runtime failures.

**Audit Logging**: All critical operations (configuration changes, connection states, command executions) are recorded as structured audit logs, supporting security compliance and troubleshooting. Logs use lumberjack for automatic rotation and compression.
