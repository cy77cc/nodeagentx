# OpsAgent

OpsAgent is a host-side execution and metrics collection agent for the OpsPilot control plane. Written in Go with an embedded Rust plugin runtime, it includes two core subsystems:

- **Telegraf-style metrics collection pipeline** -- Input -> Processor -> Aggregator -> Output plugin architecture
- **nsjail sandbox execution engine** -- Namespace isolation + cgroup v2 resource limits + security policies

Additional subsystems: Gateway (TCP tunnel + SSH proxy jump host), Checker (5 categories of system health checks), Plugin Gateway (custom plugin lifecycle management), Config hot-reload, and audit logging.

## Core Capabilities

| Capability | Description |
|------|------|
| Metrics Collection Pipeline | 20 built-in plugins (10 Input + 3 Processor + 4 Aggregator + 3 Output), label injection, aggregation, multi-output |
| Remote Command Execution | Whitelist policy, timeout control, output truncation |
| Sandbox Execution | nsjail PID/NET/MNT namespace isolation, cgroup v2 memory/CPU/PID limits |
| gRPC Bidirectional Stream | Agent initiates connection to platform, supports registration, heartbeat, metrics reporting, command/script dispatch |
| Script Execution | Run bash/python3 scripts inside sandbox with real-time streaming output |
| Rust Plugin Runtime | UDS JSON-RPC, supports log parsing, text processing, eBPF collection, etc. |
| Security Policy | Command whitelist/blacklist, script keyword interception, shell injection detection |
| Offline Buffering | Ring buffer for metrics during disconnection, automatic replay on reconnection |
| Prometheus Export | Built-in `/metrics` endpoint |
| Config Hot-Reload | Triggered by SIGHUP + gRPC ConfigUpdate, atomic rollback, supports hot-update for collector/reporter/auth/prometheus |
| Audit Logging | JSON-lines format, lumberjack rotation, covers config/plugin/task/grpc/sandbox events |
| Custom Plugin Gateway | plugin.yaml manifest discovery, health checks, auto-restart (exponential backoff), fsnotify file watching |
| Health Check | /healthz subsystem aggregation (healthy/degraded/unhealthy), /readyz readiness probe |
| CLI Subcommands | run, version, validate, plugins |
| Gateway/Jump Host | TCP tunnel relay + SSH proxy, supports NAT traversal for internal host access |
| System Health Check | 5 categories, 18 checks: kernel, filesystem, network, service, container |

## Architecture

```
+------------------------------------------------------+
|                    Platform (OpsPilot)                |
|         gRPC Server <-- Bidirectional Stream --> Agent Client          |
+----------------------------+---------------------------+
                              |
+----------------------------+---------------------------+
|                   OpsAgent Agent                     |
|                                                       |
|  +-------------+  +-------------+  +--------------+  |
|  |  Collector   |  |  Sandbox    |  |  Executor    |  |
|  |  Pipeline    |  |  (nsjail)   |  |  (local)     |  |
|  |             |  |             |  |              |  |
|  | Input -->   |  | Command/    |  | Direct       |  |
|  | Processor -->|  | Script      |  | Execution    |  |
|  | Aggregator -->|  | Isolated    |  |              |  |
|  | Output -->  |  | Execution   |  |              |  |
|  +-------------+  +-------------+  +--------------+  |
|  +-------------+  +--------------+  +--------------+  |
|                   | gRPC Client |  | Plugin Runtime|  |
|                   | Heartbeat/  |  | (Rust UDS)   |  |
|                   | Reconnect/  |  |              |  |
|                   | Buffer      |  |              |  |
|                   +-------------+  +--------------+  |
|  +-------------+  +--------------+  +--------------+  |
|  | Audit Logger |  |Config Reloader|  |Plugin Gateway |  |
|  | (JSON-lines) |  |(SIGHUP/gRPC) |  |(plugin.yaml) |  |
|  +-------------+  +--------------+  +--------------+  |
|  +-------------+  +--------------+                     |
|  |   Gateway    |  |   Checker    |                     |
|  | TCP Tunnel   |  | 5 Categories |                     |
|  | Relay        |  | Health Check |                     |
|  | SSH Proxy    |  | Kernel/FS/   |                     |
|  | NAT Traversal|  | Network/     |                     |
|  |              |  | Service/     |                     |
|  |              |  | Container    |                     |
|  +-------------+  +--------------+                     |
|  +-------------------------------------------------+  |
|  |              HTTP Server (:18080)                |  |
|  |  /healthz /readyz /api/v1/exec /api/v1/tasks    |  |
|  |  /api/v1/metrics/latest /metrics                 |  |
|  +-------------------------------------------------+  |
+-------------------------------------------------------+
```

## Built-in Plugins

### Input Plugins (Collection)

| Plugin | type | Description | Optional Config |
|------|------|------|------------|
| CPU | `cpu` | CPU usage | `per_cpu`, `total_cpu` |
| Memory | `memory` | Virtual memory + swap | -- |
| Disk | `disk` | Disk usage | `mount_points` |
| Network | `net` | Network I/O counters | -- |
| Process | `process` | Top-N processes | `top_n` |
| Disk I/O | `diskio` | Disk read/write counters | `devices` |
| Network Connections | `connections` | Network connection state statistics | `states` |
| Load | `load` | System load average (1/5/15 min) | -- |
| GPU | `gpu` | NVIDIA GPU metrics (nvidia-smi) | `bin_path` |
| Temperature | `temp` | Temperature sensors | -- |

### Processor Plugins (Processing)

| Plugin | type | Description | Config |
|------|------|------|--------|
| Tagger | `tagger` | Static/conditional label injection | `tags`, `rules` |
| Regex Replace | `regex` | Label value regex transformation | `tags[].key`, `pattern`, `replacement` |
| Delta/Rate | `delta` | Cumulative counter delta or rate calculation | `fields`, `output` (delta/rate), `max_stale_seconds` |

### Aggregator Plugins (Aggregation)

| Plugin | type | Description | Config |
|------|------|------|--------|
| Average | `avg` | Metric average | `fields`, `period` |
| Sum | `sum` | Metric accumulation | `fields`, `period` |
| Min/Max | `minmax` | Metric minimum and maximum | `fields` |
| Percentile | `percentile` | P50/P95/P99 percentiles | `fields`, `percentiles` |

### Output Plugins (Output)

| Plugin | type | Description | Config |
|------|------|------|--------|
| HTTP | `http` | JSON POST + retry | `url`, `timeout`, `retry_count` |
| Prometheus | `prometheus` | Text format exposition | `path`, `addr` |
| Prometheus Remote Write | `prometheus_remote_write` | Remote write | `url`, `timeout` |

## Quick Start

```bash
# Install dependencies
make tidy

# Run tests (with race detector)
make test-race

# Build
make build

# Run
./bin/opsagent run --config ./configs/config.yaml

# Smoke test (build + test + vet + security + sandbox check + integration)
./scripts/smoke-test.sh
```

## Configuration

See `configs/config.yaml` for a complete configuration example. Key configuration items:

```yaml
# Agent basic information
agent:
  id: "agent-001"
  name: "web-server-01"
  interval_seconds: 10        # Metrics collection interval
  audit_log:
    enabled: false
    path: "/var/log/opsagent/audit.jsonl"
    max_size_mb: 100
    max_backups: 5

# Metrics collection pipeline
collector:
  inputs:
    - type: cpu
      config: { per_cpu: false }
    - type: memory
      config: {}
    - type: disk
      config: {}
    - type: net
      config: {}
  processors:
    - type: tagger
      config: { tags: { env: "production" } }
  outputs:
    - type: http
      config: { url: "https://metrics.example.com/push" }

# gRPC platform connection
grpc:
  server_addr: "platform.example.com:443"
  enroll_token: "your-token"
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
  heartbeat_interval_seconds: 15

# Sandbox execution
sandbox:
  enabled: true
  nsjail_path: "/usr/bin/nsjail"
  policy:
    allowed_commands: [echo, ls, cat, grep, df, free]
    blocked_commands: [rm, mkfs, dd]
    allowed_interpreters: [bash, python3]
    script_max_bytes: 65536
    shell_injection_check: true

# Command executor
executor:
  timeout_seconds: 10
  allowed_commands: [uptime, df, free, hostname]

# Reporter
reporter:
  mode: "stdout"  # stdout | http

# API authentication
auth:
  enabled: false
  bearer_token: ""

# Prometheus export
prometheus:
  enabled: true
  path: "/metrics"

# Rust plugin
plugin:
  enabled: false
  runtime_path: "./rust-runtime/target/release/opsagent-rust-runtime"
  socket_path: "/tmp/opsagent/plugin.sock"

# Custom plugin gateway
plugin_gateway:
  enabled: false
  plugins_dir: "/etc/opsagent/plugins"
  startup_timeout_seconds: 10
  health_check_interval_seconds: 30
  max_restarts: 3
  restart_backoff_seconds: 5
  file_watch_debounce_seconds: 2

# Gateway/Jump host
gateway:
  enabled: false
  listen_addr: ":18081"
  max_tunnels: 100
  tunnel_timeout_seconds: 30
  idle_timeout_seconds: 300
  hosts:
    - id: "internal-db"
      addr: "10.0.1.50:22"
      mode: "auto"            # tunnel | proxy | auto
      ssh:
        user: "admin"
        port: 22
        key_file: "/etc/opsagent/keys/internal-db"
    - id: "internal-cache"
      addr: "10.0.1.51:6379"
      mode: "tunnel"

# System health check
checker:
  enabled: true
  max_concurrent: 5
  default_timeout_seconds: 30
  disabled_checkers: []       # Optional: disable specific checkers
```

## API

```bash
# Health check
curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/readyz

# Execute command
curl -X POST http://127.0.0.1:18080/api/v1/exec \
  -H 'Content-Type: application/json' \
  -d '{"command":"df","args":["-h"],"timeout_seconds":10}'

# Sandbox execute command
curl -X POST http://127.0.0.1:18080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{"task_id":"t1","type":"sandbox_exec","payload":{"command":"echo","args":["hello"]}}'

# Sandbox execute script
curl -X POST http://127.0.0.1:18080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{"task_id":"t2","type":"sandbox_exec","payload":{"interpreter":"bash","script":"df -h && free -h"}}'

# Rust plugin task
curl -X POST http://127.0.0.1:18080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{"task_id":"t3","type":"plugin_text_process","payload":{"text":"hello","operation":"uppercase"}}'

# Prometheus metrics
curl http://127.0.0.1:18080/metrics

# Latest metrics snapshot
curl http://127.0.0.1:18080/api/v1/metrics/latest
```

## Makefile Targets

| Target | Description |
|------|------|
| `make build` | Compile agent to `bin/opsagent` |
| `make build-all` | Cross-compile for amd64 + arm64 |
| `make package` | Package installation archives for both architectures (`dist/*.tar.gz`) |
| `make package-amd64` | Package amd64 only |
| `make package-arm64` | Package arm64 only |
| `make clean` | Clean bin/ dist/ coverage.out |
| `make test` | Run all tests |
| `make test-race` | Run tests with race detector |
| `make test-cover` | Test coverage report |
| `make lint` | golangci-lint |
| `make vet` | go vet static analysis |
| `make proto` | Generate Go code from proto |
| `make rust-build` | Compile Rust runtime |
| `make sandbox-check` | Check nsjail/cgroup/namespace prerequisites |
| `make integration` | Run integration tests |
| `make integration-sandbox` | Run sandbox integration tests (requires root) |
| `make security` | gosec security scan |
| `make ci` | CI pipeline (tidy + vet + test-race + security) |
| `make bench` | Collector pipeline benchmark (benchmem, count=3) |
| `make e2e` | End-to-end integration test (e2e build tag, 120s timeout) |

## Packaging & Installation

```bash
# Package (development machine)
make package
# dist/opsagent-<version>-linux-amd64.tar.gz
# dist/opsagent-<version>-linux-arm64.tar.gz

# Install (target machine)
tar xzf opsagent-<version>-linux-amd64.tar.gz
cd opsagent-<version>-linux-amd64
sudo ./install.sh

# Manage service
sudo systemctl start opsagent
sudo systemctl enable opsagent
sudo journalctl -u opsagent -f

# Uninstall
sudo ./uninstall.sh
```

## Security Boundaries

1. Command whitelist + `exec.CommandContext`, forbids `sh -c` concatenation
2. Sandbox execution: nsjail PID/NET/MNT namespace isolation
3. cgroup v2 resource limits: memory cap, CPU quota, PID count
4. Script security: keyword blacklist + shell injection detection
5. stdout/stderr output byte limit
6. Rust plugin runs in a separate subprocess + local socket for isolation and circuit-breaking
7. Optional mTLS + Bearer Token authentication
8. Sandbox seccomp syscall whitelist, only allows basic I/O/process management syscalls
9. Audit logging records all security-related events (config/plugin/task/grpc/sandbox)
10. Plugin gateway auto-restart on health check failure (exponential backoff), marks as error after limit exceeded
11. Gateway PSK authentication + SSH host key verification
12. SSH parameter escaping to prevent command injection
13. crypto/rand for tunnel ID generation (unpredictable)

> See the [Security Hardening Guide](docs/zh/security-hardening.md) for complete security hardening documentation.

## CI/CD

The project uses GitHub Actions for automation:

**CI** (`.github/workflows/ci.yml`) -- Runs on every push/PR:
- Go: `go mod tidy` + `go vet` + `golangci-lint` + `go build` + `go test -race` (with 80% coverage threshold)
- Rust: `cargo build` + `cargo test` + `cargo clippy` + `cargo audit`
- Integration: integration tests (depends on Go + Rust passing)

**Release** (`.github/workflows/release.yml`) -- Triggered by pushing `v*` tags:
- Cross-compile for amd64 + arm64
- Package tar.gz (includes binary, config, systemd service, install/uninstall scripts)
- Create GitHub Release and upload artifacts

```bash
# Trigger automated release
git tag v1.0.0
git push origin v1.0.0
```

**Local test commands:**
```bash
make test-race      # Tests + race detector
make test-cover     # Coverage report
make bench          # Benchmark
make e2e            # End-to-end tests
make integration    # Integration tests
make ci             # Full CI pipeline
```

## Platform Integration

See the [Platform Integration Guide](docs/zh/platform-integration-guide.md) for details, including:

- gRPC proto definitions and message types
- Complete platform-side Go server implementation examples
- Message interaction flow (registration, heartbeat, metrics, command execution, script execution)
- Configuration reference and troubleshooting

## Project Structure

```
OpsAgent/
+-- cmd/agent/                    # Entry point
+-- internal/
|   +-- app/                      # Agent lifecycle orchestration, audit logging, CLI subcommands
|   +-- collector/                # Collection pipeline
|   |   +-- inputs/               #   cpu, memory, disk, diskio, net, process, connections, load, gpu, temp
|   |   +-- processors/           #   tagger, regex, delta
|   |   +-- aggregators/          #   avg, sum, minmax, percentile
|   |   +-- outputs/              #   http, prometheus, promrw
|   +-- config/                   # Configuration loading and validation
|   +-- executor/                 # Local command execution
|   +-- gateway/                  # Gateway/Jump host subsystem
|   |   +-- tunnel/               #   TCP tunnel pool and tunnel management
|   |   +-- proxy/                #   SSH proxy and command relay
|   +-- checker/                  # System health check subsystem
|   |   +-- kernel/               #   Kernel version, boot parameters, modules, sysctl
|   |   +-- filesystem/           #   File existence, permissions, directory permissions, mount options
|   |   +-- network/              #   Network parameters, ports, iptables, SSH config
|   |   +-- service/              #   Service checks, user checks, PAM, cron
|   |   +-- container/            #   Docker, containerd, cgroup, container runtime
|   +-- grpcclient/               # gRPC client (connect/send/receive/buffer)
|   |   +-- proto/                #   Generated protobuf code
|   +-- health/                   # Health check interface and state definitions
|   +-- integration/              # Integration tests
|   +-- logger/                   # zerolog wrapper
|   +-- pluginruntime/            # Rust plugin runtime client
|   +-- reporter/                 # Reporting strategy (stdout/http)
|   +-- sandbox/                  # nsjail sandbox execution engine
|   +-- server/                   # HTTP API + Prometheus export
|   +-- task/                     # Task model and dispatch
+-- proto/                        # gRPC proto definitions
+-- rust-runtime/                 # Rust plugin runtime
+-- configs/config.yaml           # Default configuration
+-- scripts/
|   +-- package.sh                # Cross-compile packaging script (amd64/arm64)
|   +-- ci-package.sh             # CI packaging script (called by package.sh and Actions)
|   +-- uninstall.sh              # Uninstall script
|   +-- smoke-test.sh             # Smoke test script
|   +-- dev.sh                    # Development run script
+-- .github/workflows/
|   +-- ci.yml                    # CI: build + test + vet
|   +-- release.yml               # Release: cross-compile + package + publish
+-- docs/
|   +-- zh/                       # Chinese documentation
|   |   +-- quickstart.md             # Quick start
|   |   +-- config-reference.md       # Configuration reference
|   |   +-- api-reference.md          # API reference
|   |   +-- architecture.md           # Architecture design
|   |   +-- platform-integration-guide.md # Platform integration guide
|   |   +-- plugin-contract.md        # Plugin contract specification
|   |   +-- sdk-development-guide.md  # SDK development guide
|   |   +-- security-hardening.md     # Security hardening guide
|   |   +-- operations-guide.md       # Operations deployment guide
|   |   +-- gateway-tunnel-guide.md   # Gateway tunnel guide
|   |   +-- changelog.md              # Changelog
|   +-- en/                       # English documentation
|   +-- archive/                  # Archived documentation (development plans and design specs)
+-- Makefile
```

## Documentation

| Document | Description |
|------|------|
| [Quick Start](docs/zh/quickstart.md) | Installation, configuration, and first-run guide |
| [Configuration Reference](docs/zh/config-reference.md) | Complete configuration items with descriptions and examples |
| [API Reference](docs/zh/api-reference.md) | HTTP API endpoints and request/response formats |
| [Architecture Design](docs/zh/architecture.md) | Subsystem architecture, data flow, and design decisions |
| [Platform Integration Guide](docs/zh/platform-integration-guide.md) | Platform developer guide: gRPC service implementation, message interaction flow |
| [Plugin Contract Specification](docs/zh/plugin-contract.md) | UDS JSON-RPC 2.0 protocol definition |
| [SDK Development Guide](docs/zh/sdk-development-guide.md) | Go/Rust plugin SDK usage guide |
| [Security Hardening Guide](docs/zh/security-hardening.md) | Security architecture, hardening measures, audit configuration |
| [Operations Deployment Guide](docs/zh/operations-guide.md) | Deployment, monitoring, and troubleshooting |
| [Gateway Tunnel Guide](docs/zh/gateway-tunnel-guide.md) | Gateway configuration, tunnel management, SSH proxy |
| [CHANGELOG](docs/zh/changelog.md) | Version change history |
