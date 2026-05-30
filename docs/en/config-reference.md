# OpsAgent Configuration Reference

This document is the complete reference for the OpsAgent YAML configuration file. The configuration file uses YAML format and the path is specified via the `--config` command-line argument.

---

## Table of Contents

1. [agent -- Agent Identity and Collection Interval](#1-agent)
2. [server -- Local API Service](#2-server)
3. [executor -- Command Execution Limits](#3-executor)
4. [reporter -- Data Reporting](#4-reporter)
5. [auth -- API Authentication](#5-auth)
6. [prometheus -- Metrics Export](#6-prometheus)
7. [plugin -- Rust Plugin Runtime](#7-plugin)
8. [grpc -- gRPC Platform Connection](#8-grpc)
9. [sandbox -- nsjail Sandbox Execution](#9-sandbox)
10. [collector -- Metric Collection Pipeline](#10-collector)
11. [plugin_gateway -- Custom Plugin Gateway](#11-plugin_gateway)
12. [checker -- System Health Checks](#12-checker)
13. [gateway -- Tunnel/Proxy Gateway](#13-gateway)
14. [discovery -- Service Auto-Discovery](#14-discovery)
15. [updater -- Auto-Update](#15-updater)
16. [wasm -- WASM Plugin Runtime](#16-wasm)
17. [Complete Configuration Example](#complete-configuration-example)

---

## 1. agent

Controls agent identity and data collection interval.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `agent.id` | string | -- | Yes | Unique agent identifier, used to distinguish different agent instances on the platform |
| `agent.name` | string | -- | Yes | Human-readable agent name for identification in the management interface |
| `agent.interval_seconds` | int | `10` | Yes | Data collection interval in seconds, must be greater than 0 |
| `agent.shutdown_timeout_seconds` | int | `30` | No | Graceful shutdown wait timeout in seconds, forced exit after timeout |

### agent.audit_log

Agent-level audit log configuration.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `agent.audit_log.enabled` | bool | `false` | No | Whether to enable audit logging |
| `agent.audit_log.path` | string | `/var/log/opsagent/audit.jsonl` | No | Audit log file path |
| `agent.audit_log.max_size_mb` | int | `100` | No | Maximum size of a single log file in MB, rotated when exceeded |
| `agent.audit_log.max_backups` | int | `5` | No | Number of historical log files to retain |

---

## 2. server

Controls local HTTP API service settings.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `server.listen_addr` | string | `127.0.0.1:18080` | Yes | HTTP API listen address, format is `host:port` |

---

## 3. executor

Controls security boundaries for command execution.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `executor.timeout_seconds` | int | `10` | Yes | Single command execution timeout in seconds, must be greater than 0 |
| `executor.allowed_commands` | array[string] | -- | Yes | Allowlist of commands permitted to execute, cannot be empty, each entry cannot be an empty string |
| `executor.max_output_bytes` | int | `65536` | Yes | Maximum command output size in bytes (default 64KB), must be greater than 0 |

---

## 4. reporter

Controls how collected data is reported.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `reporter.mode` | string | `stdout` | Yes | Reporting mode, options: `stdout` (standard output), `http` (HTTP push) |
| `reporter.endpoint` | string | -- | Conditionally required | HTTP reporting address, required when `reporter.mode=http` |
| `reporter.timeout_seconds` | int | `5` | Yes | Reporting request timeout in seconds, must be greater than 0 |
| `reporter.retry_count` | int | `3` | No | Number of retries on reporting failure, must be greater than or equal to 0 |
| `reporter.retry_interval_ms` | int | `500` | No | Retry interval in milliseconds, must be greater than or equal to 0 |

---

## 5. auth

Controls the authentication mechanism for API endpoints.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `auth.enabled` | bool | `true` | No | Whether to enable Bearer Token authentication |
| `auth.bearer_token` | string | -- | Conditionally required | Authentication token, required when `auth.enabled=true`, must be at least 32 characters |

> **Security Notice**: Always enable authentication and set a strong random token in production environments. Startup will fail if the token is less than 32 characters.

---

## 6. prometheus

Controls the Prometheus metrics export endpoint.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `prometheus.enabled` | bool | `true` | No | Whether to enable the Prometheus metrics endpoint |
| `prometheus.path` | string | `/metrics` | Conditionally required | Metrics endpoint URL path, must start with `/` when enabled |
| `prometheus.protect_with_auth` | bool | `false` | No | Whether to reuse the `auth` configuration to protect the metrics endpoint |

---

## 7. plugin

Controls Rust plugin runtime integration.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `plugin.enabled` | bool | `false` | No | Whether to enable the Rust plugin runtime |
| `plugin.runtime_path` | string | `./rust-runtime/target/release/github.com/cy77cc/opsagent-rust-runtime` | Conditionally required | Rust runtime binary path, required when `auto_start=true` |
| `plugin.socket_path` | string | `/tmp/github.com/cy77cc/opsagent/plugin.sock` | Conditionally required | Unix Domain Socket path, required when enabled |
| `plugin.auto_start` | bool | `true` | No | Whether to automatically start the runtime process when the agent starts |
| `plugin.startup_timeout_seconds` | int | `5` | Yes | Runtime startup timeout in seconds, must be greater than 0 |
| `plugin.request_timeout_seconds` | int | `30` | Yes | Single plugin request timeout in seconds, must be greater than 0 |
| `plugin.max_concurrent_tasks` | int | `4` | Yes | Maximum concurrent plugin tasks, must be greater than 0 |
| `plugin.max_result_bytes` | int | `8388608` | Yes | Maximum plugin result size in bytes (default 8MB), must be greater than 0 |
| `plugin.chunk_size_bytes` | int | `262144` | Yes | Chunk size for large result transfer (default 256KB), must be greater than 0 |
| `plugin.sandbox_profile` | string | `strict` | Yes | Plugin sandbox policy, such as `strict`, `relaxed`, etc. |

---

## 8. grpc

Controls the gRPC connection to the management platform.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `grpc.server_addr` | string | -- | Yes | gRPC server address, format is `host:port` |
| `grpc.enroll_token` | string | -- | No | Agent enrollment token, used during initial registration |
| `grpc.heartbeat_interval_seconds` | int | `15` | Yes | Heartbeat sending interval in seconds, must be greater than 0 |
| `grpc.reconnect_initial_backoff_ms` | int | `1000` | Yes | Initial backoff time for reconnection in milliseconds, must be greater than 0 |
| `grpc.reconnect_max_backoff_ms` | int | `30000` | Yes | Maximum backoff time for reconnection in milliseconds, must be greater than 0 |
| `grpc.cache_persist_path` | string | `""` | No | Local cache persistence path, empty means memory-only caching |

### grpc.mtls

Mutual TLS certificate configuration.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `grpc.mtls.cert_file` | string | `""` | No | Client certificate file path |
| `grpc.mtls.key_file` | string | `""` | No | Client private key file path |
| `grpc.mtls.ca_file` | string | `""` | No | CA certificate file path, used to verify the server |

---

## 9. sandbox

Controls the nsjail-based command sandbox execution environment.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `sandbox.enabled` | bool | `false` | No | Whether to enable sandbox execution |
| `sandbox.nsjail_path` | string | -- | Conditionally required | nsjail binary file path, required when enabled |
| `sandbox.base_workdir` | string | -- | Conditionally required | Sandbox working directory root path, required when enabled |
| `sandbox.default_timeout_seconds` | int | `30` | Yes | Default timeout for commands inside the sandbox in seconds, must be greater than 0 |
| `sandbox.max_concurrent_tasks` | int | `4` | Yes | Maximum concurrent sandbox tasks, must be greater than 0 |
| `sandbox.cgroup_base_path` | string | -- | Conditionally required | cgroup mount base path, required when enabled |
| `sandbox.audit_log_path` | string | -- | Conditionally required | Sandbox audit log path, required when enabled |
| `sandbox.allow_unsandboxed_fallback` | bool | `false` | No | Whether to fall back to non-sandbox execution when the sandbox is unavailable |

### sandbox.policy

Sandbox security policy.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `sandbox.policy.allowed_commands` | array[string] | -- | Conditionally required | List of commands allowed to execute in the sandbox, cannot be empty when enabled |
| `sandbox.policy.blocked_commands` | array[string] | `[]` | No | List of commands explicitly prohibited from execution |
| `sandbox.policy.blocked_keywords` | array[string] | `[]` | No | Keywords prohibited in commands (e.g., `"rm -rf /"`) |
| `sandbox.policy.allowed_interpreters` | array[string] | `[]` | No | Allowed script interpreters (e.g., `bash`, `python3`) |
| `sandbox.policy.script_max_bytes` | int | `65536` | Yes | Maximum script content size in bytes (default 64KB), must be greater than 0 |
| `sandbox.policy.shell_injection_check` | bool | `true` | No | Whether to enable shell injection detection |

---

## 10. collector

Defines the metric collection pipeline, consisting of four stages: input, processor, aggregator, and output. Each stage is a list of plugin instances.

### Common Fields

Each plugin instance contains:

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | Plugin type identifier |
| `config` | object | Configuration parameters specific to this plugin type |

### collector.inputs

List of data input sources.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `collector.inputs` | array[PluginInstanceConfig] | -- | No | List of input plugins |

Common input types: `cpu`, `memory`, `disk`, `net`, `load`, `diskio`, `temp`, `gpu`, `connections`.

### collector.processors

List of data processors.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `collector.processors` | array[PluginInstanceConfig] | `[]` | No | List of processor plugins, used for data transformation (e.g., delta/rate calculation) |

### collector.aggregators

List of data aggregators.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `collector.aggregators` | array[PluginInstanceConfig] | `[]` | No | List of aggregator plugins, used for statistical aggregation (e.g., min/max, percentile) |

### collector.outputs

List of data output targets.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `collector.outputs` | array[PluginInstanceConfig] | `[]` | No | List of output plugins, defining where data is sent |

---

## 11. plugin_gateway

Manages the discovery, startup, and lifecycle of custom plugins.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `plugin_gateway.enabled` | bool | `false` | No | Whether to enable the plugin gateway |
| `plugin_gateway.plugins_dir` | string | `/etc/opsagent/plugins` | Conditionally required | Plugin directory path, required when enabled |
| `plugin_gateway.startup_timeout_seconds` | int | `10` | Yes | Plugin startup timeout in seconds, must be greater than 0 |
| `plugin_gateway.health_check_interval_seconds` | int | `30` | Yes | Health check interval in seconds, must be greater than 0 |
| `plugin_gateway.max_restarts` | int | `3` | No | Maximum plugin restart count, must be greater than or equal to 0 |
| `plugin_gateway.restart_backoff_seconds` | int | `5` | No | Restart backoff time in seconds |
| `plugin_gateway.file_watch_debounce_seconds` | int | `2` | No | File change watch debounce time in seconds |
| `plugin_gateway.plugin_configs` | object | -- | No | Individual plugin configurations, key is plugin name, value is configuration key-value pairs |

---

## 12. checker

Controls the system health check subsystem.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `checker.enabled` | bool | `true` | No | Whether to enable health checks |
| `checker.max_concurrent` | int | `5` | Yes | Maximum concurrent checks, must be greater than 0 when enabled |
| `checker.default_timeout_seconds` | int | `30` | Yes | Default timeout for individual checks in seconds, must be greater than 0 when enabled |
| `checker.disabled_checkers` | array[string] | `[]` | No | List of disabled checker names |

---

## 13. gateway

Controls the tunnel/proxy gateway subsystem, used for jump server scenarios.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `gateway.enabled` | bool | `false` | No | Whether to enable the gateway |
| `gateway.listen_addr` | string | `:18081` | Conditionally required | Gateway listen address, required when enabled |
| `gateway.max_tunnels` | int | `100` | Yes | Maximum number of tunnels, must be greater than 0 when enabled |
| `gateway.tunnel_timeout_seconds` | int | `30` | Yes | Tunnel establishment timeout in seconds, must be greater than 0 when enabled |
| `gateway.idle_timeout_seconds` | int | `300` | Yes | Idle tunnel recycling timeout in seconds, must be greater than 0 when enabled |
| `gateway.hosts` | array[GatewayHostConfig] | `[]` | No | List of internal network hosts |

### gateway.hosts[]

Defines internal network hosts behind the gateway.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `gateway.hosts[].id` | string | -- | Yes | Unique host identifier |
| `gateway.hosts[].addr` | string | -- | Yes | Host address, format is `host:port` |
| `gateway.hosts[].mode` | string | -- | Yes | Connection mode, options: `tunnel` (tunnel), `proxy` (proxy), `auto` (automatic selection) |
| `gateway.hosts[].ssh.user` | string | -- | Conditionally required | SSH username, required when `mode=proxy` or `mode=auto` |
| `gateway.hosts[].ssh.password` | string | -- | No | SSH password (key-based authentication is recommended) |
| `gateway.hosts[].ssh.key_file` | string | -- | No | SSH private key file path |
| `gateway.hosts[].ssh.port` | int | -- | Conditionally required | SSH port, required when `mode=proxy` or `mode=auto`, must be greater than 0 |

---

## 14. discovery

Controls the service auto-discovery subsystem.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `discovery.enabled` | bool | `false` | No | Whether to enable service auto-discovery |
| `discovery.interval_seconds` | int | `60` | Yes | Discovery scan interval in seconds, must be greater than 0 when enabled |
| `discovery.layers` | array[DiscoveryLayerConfig] | `[]` | No | List of discovery layers to enable |

### discovery.layers[]

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `discovery.layers[].name` | string | -- | Yes | Layer name: `systemd`, `proc`, `container`, `metadata` |
| `discovery.layers[].enabled` | bool | `true` | No | Whether this layer is enabled |

---

## 15. updater

Controls the auto-update subsystem.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `updater.enabled` | bool | `false` | No | Whether to enable auto-update |
| `updater.current_path` | string | -- | Conditionally required | Path to the current agent binary, required when enabled |
| `updater.backup_path` | string | -- | Conditionally required | Path to store the backup binary, required when enabled |
| `updater.download_dir` | string | -- | Conditionally required | Temporary directory for downloading updates, required when enabled |
| `updater.public_key` | string | -- | Conditionally required | Ed25519 public key (hex-encoded) for signature verification, required when enabled |

---

## 16. wasm

Controls the WASM plugin runtime.

| YAML Path | Type | Default | Required | Description |
|-----------|------|---------|----------|-------------|
| `wasm.enabled` | bool | `false` | No | Whether to enable WASM plugin runtime |
| `wasm.plugins_dir` | string | `/etc/opsagent/wasm-plugins` | Conditionally required | Directory containing WASM plugin manifests and binaries, required when enabled |
| `wasm.max_modules` | int | `10` | Yes | Maximum number of WASM modules to load, must be greater than 0 when enabled |
| `wasm.cache_dir` | string | `/var/lib/opsagent/wasm-cache` | No | Directory for caching compiled WASM modules |

---

## 17. Complete Configuration Example

```yaml
# ============================================================
# OpsAgent Complete Configuration Example
# ============================================================

# 1. Agent identity and collection interval
agent:
  id: "agent-local-001"
  name: "local-dev-agent"
  interval_seconds: 10
  shutdown_timeout_seconds: 30
  audit_log:
    enabled: false
    path: "/var/log/opsagent/audit.jsonl"
    max_size_mb: 100
    max_backups: 5

# 2. Local API service
server:
  listen_addr: "127.0.0.1:18080"

# 3. Command execution limits
executor:
  timeout_seconds: 10
  max_output_bytes: 65536
  allowed_commands:
    - uptime
    - df
    - free
    - whoami
    - hostname
    - ip
    - ss

# 4. Data reporting
reporter:
  mode: "stdout"          # stdout | http
  endpoint: ""
  timeout_seconds: 5
  retry_count: 3
  retry_interval_ms: 500

# 5. API authentication
auth:
  enabled: false
  bearer_token: ""        # Must set a 32+ character token when enabled

# 6. Prometheus metrics export
prometheus:
  enabled: true
  path: "/metrics"
  protect_with_auth: false

# 7. Rust plugin runtime
plugin:
  enabled: false
  runtime_path: "./rust-runtime/target/release/opsagent-rust-runtime"
  socket_path: "/tmp/opsagent/plugin.sock"
  auto_start: true
  startup_timeout_seconds: 5
  request_timeout_seconds: 30
  max_concurrent_tasks: 4
  max_result_bytes: 8388608
  chunk_size_bytes: 262144
  sandbox_profile: "strict"

# 8. gRPC platform connection
grpc:
  server_addr: "platform.example.com:443"
  enroll_token: ""
  mtls:
    cert_file: ""
    key_file: ""
    ca_file: ""
  heartbeat_interval_seconds: 15
  reconnect_initial_backoff_ms: 1000
  reconnect_max_backoff_ms: 30000
  cache_persist_path: ""

# 9. nsjail sandbox execution
sandbox:
  enabled: false
  nsjail_path: "/usr/bin/nsjail"
  base_workdir: "/tmp/opsagent/sandbox"
  default_timeout_seconds: 30
  max_concurrent_tasks: 4
  cgroup_base_path: "/sys/fs/cgroup/opsagent"
  audit_log_path: "/var/log/opsagent/audit.log"
  allow_unsandboxed_fallback: false
  policy:
    allowed_commands:
      - echo
      - ls
      - cat
      - grep
      - wc
    blocked_commands:
      - rm
      - mkfs
      - dd
    blocked_keywords:
      - "rm -rf /"
    allowed_interpreters:
      - bash
      - python3
    script_max_bytes: 65536
    shell_injection_check: true

# 10. Metric collection pipeline
collector:
  inputs:
    - type: cpu
      config:
        per_cpu: false
    - type: memory
      config: {}
    - type: disk
      config: {}
    - type: net
      config: {}
    - type: load
      config: {}
    - type: diskio
      config: {}
    - type: temp
      config: {}
    - type: gpu
      config: {}
    - type: connections
      config: {}
  processors: []
  aggregators: []
  outputs: []

# 11. Custom plugin gateway
plugin_gateway:
  enabled: false
  plugins_dir: "/etc/opsagent/plugins"
  startup_timeout_seconds: 10
  health_check_interval_seconds: 30
  max_restarts: 3
  restart_backoff_seconds: 5
  file_watch_debounce_seconds: 2
  plugin_configs: {}

# 12. System health checks
checker:
  enabled: true
  max_concurrent: 5
  default_timeout_seconds: 30
  disabled_checkers: []

# 13. Tunnel/proxy gateway
gateway:
  enabled: false
  listen_addr: ":18081"
  max_tunnels: 100
  tunnel_timeout_seconds: 30
  idle_timeout_seconds: 300
  hosts: []
  # Example host configuration:
  # hosts:
  #   - id: "vm-web-01"
  #     addr: "192.168.122.100:22"
  #     mode: "auto"
  #     ssh:
  #       user: "root"
  #       key_file: "/etc/opsagent/keys/id_rsa"
  #       port: 22

# 14. Service auto-discovery
discovery:
  enabled: false
  interval_seconds: 60
  layers:
    - name: systemd
      enabled: true
    - name: proc
      enabled: true
    - name: container
      enabled: true
    - name: metadata
      enabled: true

# 15. Auto-update
updater:
  enabled: false
  current_path: ""
  backup_path: ""
  download_dir: ""
  public_key: ""

# 16. WASM plugin runtime
wasm:
  enabled: false
  plugins_dir: "/etc/opsagent/wasm-plugins"
  max_modules: 10
  cache_dir: "/var/lib/opsagent/wasm-cache"
```
