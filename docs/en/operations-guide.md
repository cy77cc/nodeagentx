# OpsAgent Operations and Deployment Guide

This guide is intended for operations engineers who use and maintain OpsAgent, covering system requirements, installation and deployment, configuration management, daily operations, and troubleshooting. OpsAgent is a host-side metric collection and sandbox execution agent built in Go, with support for Rust plugin runtime extensions.

---

## 1. System Requirements

| Item | Requirement |
|------|------|
| Operating System | Linux (amd64 / arm64) |
| Go | 1.21+ (compile time only) |
| Rust | 1.75+ (compile time for Rust runtime only) |
| nsjail | Optional, for sandbox execution |
| cgroup v2 | Optional, for sandbox resource limits |

> **Pre-check**: Run `make sandbox-check` to verify the sandbox environment is ready.

---

## 2. Installation

### 2.1 Install from Release

```bash
# Download the architecture-specific archive from GitHub Releases
tar xzf opsagent-<version>-linux-amd64.tar.gz
cd opsagent-<version>-linux-amd64
sudo ./install.sh
```

`install.sh` performs the following operations:

| Installation Item | Path |
|--------|------|
| Main binary | `/usr/local/bin/opsagent` |
| Configuration file | `/etc/opsagent/config.yaml` (existing files are preserved, new version saved as `.new`) |
| systemd service file | `/etc/systemd/system/opsagent.service` |
| Log directory | `/var/log/opsagent/` |

### 2.2 Build from Source

```bash
git clone <repo-url> opsagent
cd opsagent

make build          # Build for current architecture
make build-all      # Cross-compile for amd64 + arm64
make rust-build     # Build Rust runtime (optional)
```

Build artifacts are located in the `bin/` directory.

### 2.3 systemd Service Management

```bash
# Start service
sudo systemctl start opsagent

# Stop service
sudo systemctl stop opsagent

# Restart service
sudo systemctl restart opsagent

# View service status
sudo systemctl status opsagent

# Enable auto-start on boot
sudo systemctl enable opsagent

# Disable auto-start on boot
sudo systemctl disable opsagent

# View logs in real-time
sudo journalctl -u opsagent -f
```

**Service Security Features**:

- `Restart=always`: Automatic restart after process abnormal exit
- `After=network-online.target`: Wait for network readiness before starting
- `ProtectSystem=strict`: Mount system directories as read-only
- `ProtectHome=true`: Prohibit access to user home directories
- `PrivateTmp=true`: Use independent /tmp namespace
- `NoNewPrivileges=true`: Prohibit process privilege escalation

---

## 3. Configuration Management

### 3.1 Configuration File

Configuration file path: `/etc/opsagent/config.yaml`

Below is the complete configuration field reference:

```yaml
agent:
  id: "agent-001"                    # Required, agent unique identifier
  name: "web-server-01"              # Required, human-readable name
  interval_seconds: 10               # Metric collection interval (seconds)
  shutdown_timeout_seconds: 30       # Graceful shutdown timeout (seconds)
  audit_log:
    enabled: false                   # Enable audit log
    path: "/var/log/opsagent/audit.jsonl"
    max_size_mb: 100                 # Max size per audit log file (MB)
    max_backups: 5                   # Number of old log files to retain

server:
  listen_addr: "127.0.0.1:18080"     # HTTP API listen address

executor:
  timeout_seconds: 10                # Command execution timeout (seconds)
  max_output_bytes: 65536            # Max command output bytes
  allowed_commands: [uptime, df, free, hostname]  # Allowed command whitelist

reporter:
  mode: "stdout"                     # Reporting mode: stdout | http
  endpoint: ""                       # HTTP reporting address (required when mode=http)
  timeout_seconds: 5                 # Reporting timeout (seconds)
  retry_count: 3                     # Reporting retry count
  retry_interval_ms: 500             # Reporting retry interval (milliseconds)

auth:
  enabled: true                      # Enable authentication
  bearer_token: ""                   # Bearer Token (must be 32+ characters in production)

prometheus:
  enabled: true                      # Expose Prometheus metrics
  path: "/metrics"                   # Metrics endpoint path
  protect_with_auth: false           # Require auth for metrics endpoint

grpc:
  server_addr: "platform.example.com:443"  # Platform gRPC server address
  enroll_token: ""                         # Enrollment token
  mtls:
    cert_file: ""                    # Client certificate path
    key_file: ""                     # Client private key path
    ca_file: ""                      # CA certificate path
  heartbeat_interval_seconds: 15     # Heartbeat interval (seconds)
  reconnect_initial_backoff_ms: 1000 # Reconnect initial backoff (milliseconds)
  reconnect_max_backoff_ms: 30000    # Reconnect max backoff (milliseconds)

collector:
  inputs:                            # Collector input plugin list
    - type: cpu
      config: { per_cpu: false }
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
  processors: []                     # Data processor list
  aggregators: []                    # Data aggregator list
  outputs: []                        # Data output list

sandbox:
  enabled: false                     # Enable sandbox execution
  nsjail_path: "/usr/bin/nsjail"     # nsjail binary path
  base_workdir: "/tmp/opsagent/sandbox"  # Sandbox working directory
  default_timeout_seconds: 30        # Default execution timeout (seconds)
  max_concurrent_tasks: 4            # Max concurrent sandbox tasks
  cgroup_base_path: "/sys/fs/cgroup/opsagent"  # cgroup base path
  audit_log_path: "/var/log/opsagent/audit.log"
  policy:
    allowed_commands: [echo, ls, cat, grep, wc]  # Allowed commands in sandbox
    blocked_commands: [rm, mkfs, dd]              # Blocked commands in sandbox
    allowed_interpreters: [bash, python3]         # Allowed script interpreters
    script_max_bytes: 65536                       # Max script size in bytes
    shell_injection_check: true                   # Enable shell injection check

plugin:
  enabled: false                     # Enable Rust plugin runtime
  runtime_path: "./rust-runtime/target/release/opsagent-rust-runtime"
  socket_path: "/tmp/opsagent/plugin.sock"  # Plugin communication Unix Socket path

plugin_gateway:
  enabled: false                     # Enable plugin gateway
  plugins_dir: "/etc/opsagent/plugins"     # Plugin directory
  startup_timeout_seconds: 10        # Plugin startup timeout (seconds)
  health_check_interval_seconds: 30  # Plugin health check interval (seconds)
  max_restarts: 3                    # Max plugin restart count
  restart_backoff_seconds: 5         # Restart backoff (seconds)
```

### 3.2 Hot Reload

Supports configuration hot reload via SIGHUP signal or gRPC ConfigUpdate command.

```bash
# Trigger hot reload via signal
kill -HUP $(pidof opsagent)
```

**Hot Reload Scope**:

| Hot Reloadable (No Restart Required) | Requires Restart |
|----------------------|----------|
| collector (inputs / processors / aggregators / outputs) | agent.id |
| reporter | agent.name |
| auth | server.listen_addr |
| prometheus | grpc related configuration |
| - | sandbox related configuration |
| - | plugin related configuration |

- If non-reloadable fields are changed, the reload request will be rejected.
- On reload failure, automatically rolls back to the previous valid configuration (atomic operation).

---

## 4. Health Check and Monitoring

### 4.1 HTTP Endpoints

| Endpoint | Method | Description |
|------|------|------|
| `/healthz` | GET | Comprehensive health check, returns subsystem status: `healthy` / `degraded` / `unhealthy` |
| `/readyz` | GET | Readiness probe, can be used for Kubernetes readinessProbe |

```bash
# Check health status
curl http://127.0.0.1:18080/healthz

# Check readiness status
curl http://127.0.0.1:18080/readyz
```

### 4.2 Prometheus Metrics

When Prometheus is enabled, exposes standard format metrics data via the `/metrics` endpoint.

```bash
curl http://127.0.0.1:18080/metrics
```

This address can be configured in Prometheus's `scrape_configs` as a scrape target.

### 4.3 Audit Log

Audit logs are written in JSON Lines (JSONL) format, with one event record per line.

Supported event types:

| Event Type | Description |
|----------|------|
| `config` | Configuration change event |
| `plugin` | Plugin lifecycle event |
| `task` | Sandbox task execution event |
| `grpc` | gRPC communication event |
| `sandbox` | Sandbox isolation related event |

Log rotation is managed by lumberjack, controlled via `audit_log.max_size_mb` and `audit_log.max_backups`.

---

## 5. Daily Operations

### 5.1 Log Management

OpsAgent uses zerolog for structured log output, with log rotation via lumberjack.

```bash
# View service logs in real-time
sudo journalctl -u opsagent -f

# View last 100 lines of logs
sudo journalctl -u opsagent -n 100

# Filter logs by time range
sudo journalctl -u opsagent --since "2024-01-01" --until "2024-01-02"
```

Log rotation parameters:

- `max_size_mb`: Max size per log file (MB), rotation triggered when reached
- `max_backups`: Number of old log files to retain

### 5.2 Performance Tuning

| Parameter | Description | Tuning Recommendation |
|------|------|----------|
| `agent.interval_seconds` | Metric collection frequency | Adjust based on business needs; too low increases system overhead |
| `sandbox.max_concurrent_tasks` | Concurrent sandbox task count | Adjust based on CPU core count and memory size |
| `collector.inputs` | Collection input list | Only enable inputs that are actually needed, reduce unnecessary system calls |

### 5.3 Upgrade Procedure

```bash
# 1. Backup current configuration
cp /etc/opsagent/config.yaml /etc/opsagent/config.yaml.bak

# 2. Stop service
sudo systemctl stop opsagent

# 3. Replace binary
sudo cp opsagent /usr/local/bin/opsagent

# 4. Start service
sudo systemctl start opsagent

# 5. Verify service status
curl http://127.0.0.1:18080/healthz
```

> **Note**: If the new version includes configuration format changes, please refer to the Release Notes for configuration migration. Changes to non-hot-reloadable fields in the configuration file require a service restart to take effect.

---

## 6. Troubleshooting

### 6.1 Common Issues

| Problem | Possible Cause | Troubleshooting Method |
|------|----------|----------|
| gRPC connection failure | Network unreachable, certificate misconfiguration, wrong server address | Check network connectivity, mTLS certificates, `grpc.server_addr` configuration |
| Sandbox startup failure | nsjail not installed, cgroup v2 not enabled, insufficient permissions | Run `make sandbox-check`, check nsjail path and permissions |
| Plugin not responding | plugin.yaml misconfiguration, incorrect binary path, insufficient Socket permissions | Check plugin configuration file, binary path, Socket file permissions |
| Metric data empty | collector.inputs not configured, collection interval too large | Check `collector.inputs` configuration and `interval_seconds` value |

### 6.2 Diagnostic Commands

```bash
# Smoke test
./scripts/smoke-test.sh

# Check sandbox environment prerequisites
make sandbox-check

# Check service health status
curl http://127.0.0.1:18080/healthz

# Check service readiness status
curl http://127.0.0.1:18080/readyz

# View Prometheus metrics
curl http://127.0.0.1:18080/metrics

# Validate configuration file syntax
./bin/opsagent validate --config /etc/opsagent/config.yaml

# List loaded plugins
./bin/opsagent plugins --config /etc/opsagent/config.yaml

# Security scan
make security

# Local full CI pipeline
make ci
```

---

## 7. Uninstallation

```bash
sudo ./uninstall.sh
```

The uninstall script performs the following operations:

1. Stop and disable the opsagent service
2. Remove the `/usr/local/bin/opsagent` binary
3. Remove the `/etc/systemd/system/opsagent.service` service file
4. Interactively ask whether to also delete the configuration files (`/etc/opsagent/`) and log directory (`/var/log/opsagent/`)

---

## 8. CI/CD Integration

### 8.1 GitHub Actions CI Pipeline

The CI pipeline includes the following stages:

**Go Stages**:
- `go mod tidy` — Check dependency consistency
- `go vet` — Static analysis
- `golangci-lint` — Code style and quality check
- `go build` — Build verification
- `go test -race` — Run tests (80% coverage threshold)

**Rust Stages**:
- `cargo build` — Build Rust runtime
- `cargo test` — Run Rust tests
- `cargo clippy` — Code quality check
- `cargo audit` — Dependency security audit

**Integration Tests**:
- End-to-end functional verification

### 8.2 Release Process

Releases are triggered by Git Tags. After pushing a `v*` format Tag, cross-compilation, packaging, and publishing to GitHub Releases are automatically executed.

```bash
# Create version tag
git tag v1.0.0

# Push tag to trigger release pipeline
git push origin v1.0.0
```

The release pipeline automatically completes:
1. Cross-compile for amd64 and arm64 architectures
2. Generate archives (containing binary, install.sh, default configuration)
3. Create GitHub Release and upload artifacts
