# OpsAgent Quick Start

This guide helps you compile, configure, and run OpsAgent in a local environment, completing your first metric collection and sandbox execution.

## 1. Prerequisites

| Dependency | Version Requirement | Purpose |
|------------|-------------------|---------|
| Go | 1.26+ | Compilation |
| Git | Any | Source code retrieval |
| Linux (Ubuntu 22.04+) | - | Runtime environment |
| nsjail | 3.0+ (optional) | Sandbox execution |
| cgroup v2 | Kernel 5.2+ (optional) | Sandbox resource limits |

Check Go version:

```bash
go version
# go version go1.26.1 linux/amd64
```

Check sandbox prerequisites (optional):

```bash
# Check if nsjail is installed
which nsjail

# Check cgroup v2
test -f /sys/fs/cgroup/cgroup.controllers && echo "cgroup v2: OK" || echo "cgroup v2: unavailable"
```

## 2. Get Source Code

```bash
git clone https://github.com/cy77cc/opsagent.git
cd opsagent
```

## 3. Build

```bash
# Sync dependencies
make tidy

# Build
make build
```

The build artifact is located at `bin/opsagent`. Verify the build result:

```bash
./bin/opsagent --help
```

> If you need to cross-compile binaries for both amd64 and arm64 architectures, use `make build-all`.

## 4. Configuration

Copy the default configuration file as a starting point:

```bash
cp configs/config.yaml my-config.yaml
```

**Minimum required fields** (must be provided at startup, otherwise the process will exit with an error):

```yaml
agent:
  id: "agent-local-001"        # Unique identifier
  name: "local-dev-agent"      # Human-readable name

server:
  listen_addr: "127.0.0.1:18080"  # Local API listen address

executor:
  timeout_seconds: 10
  allowed_commands:
    - uptime
    - df
    - free

grpc:
  server_addr: "platform.example.com:443"  # Platform gRPC address
```

> `agent.id`, `agent.name`, `server.listen_addr`, `executor.allowed_commands` (non-empty), and `grpc.server_addr` are all required. Other fields have reasonable default values and do not need to be set immediately.

## 5. Run

```bash
./bin/opsagent run --config my-config.yaml
```

After successful startup, the log will display output similar to:

```
level=INFO msg="agent started" agent_id=agent-local-001 listen=127.0.0.1:18080
```

You can also run directly via Makefile (using the default configuration):

```bash
make run
```

## 6. Verify the Service

In another terminal, execute:

```bash
# Health check
curl -s http://127.0.0.1:18080/healthz

# View Prometheus metrics
curl -s http://127.0.0.1:18080/metrics
```

If `healthz` returns 200 and `metrics` has Prometheus format output, the service is running normally.

## 7. First Metric Collection

Edit `my-config.yaml` to configure the `inputs` section of the collector:

```yaml
collector:
  inputs:
    - type: cpu
      config:
        per_cpu: false
    - type: memory
      config: {}
  processors: []
  aggregators: []
  outputs: []
```

Restart OpsAgent:

```bash
# Stop the current process (Ctrl+C), then restart
./bin/opsagent run --config my-config.yaml
```

Wait for at least one collection cycle (default 10 seconds), then query the latest collected metrics:

```bash
curl -s http://127.0.0.1:18080/api/v1/metrics/latest | python3 -m json.tool
```

You should see JSON output similar to the following, containing CPU usage and memory usage:

```json
{
  "metrics": [
    {
      "name": "cpu",
      "fields": {
        "usage_percent": 12.5
      },
      "tags": {},
      "timestamp": "2026-05-25T10:30:00Z"
    },
    {
      "name": "memory",
      "fields": {
        "total_bytes": 8589934592,
        "used_bytes": 4294967296,
        "usage_percent": 50.0
      },
      "tags": {},
      "timestamp": "2026-05-25T10:30:00Z"
    }
  ]
}
```

All available input types:

| Type | Description |
|------|-------------|
| `cpu` | CPU usage |
| `memory` | Memory usage |
| `disk` | Disk usage |
| `net` | Network traffic |
| `load` | System load |
| `diskio` | Disk I/O |
| `temp` | Temperature sensors |
| `gpu` | GPU metrics (requires nvidia-smi) |
| `connections` | Network connection states |

## 8. First Sandbox Execution

> **Note**: Sandbox execution requires nsjail to be installed and OpsAgent to be running with root privileges.

Edit `my-config.yaml` to enable the sandbox:

```yaml
sandbox:
  enabled: true
  nsjail_path: "/usr/bin/nsjail"
  base_workdir: "/tmp/opsagent/sandbox"
  default_timeout_seconds: 30
  max_concurrent_tasks: 4
  cgroup_base_path: "/sys/fs/cgroup/opsagent"
  audit_log_path: "/var/log/opsagent/audit.log"
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
```

Restart as root:

```bash
sudo ./bin/opsagent run --config my-config.yaml
```

Submit a sandbox task:

```bash
curl -s -X POST http://127.0.0.1:18080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "command": "echo",
    "args": ["hello from sandbox"],
    "timeout_seconds": 10
  }' | python3 -m json.tool
```

Expected response:

```json
{
  "task_id": "task-xxxx",
  "status": "completed",
  "stdout": "hello from sandbox\n",
  "stderr": "",
  "exit_code": 0
}
```

## 9. Connect to Platform (gRPC)

Registering OpsAgent with the OpsPilot platform requires configuring a gRPC connection. It is recommended to enable mTLS in production environments.

```yaml
grpc:
  server_addr: "platform.example.com:443"
  enroll_token: "your-enrollment-token-from-platform"
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
  heartbeat_interval_seconds: 15
  reconnect_initial_backoff_ms: 1000
  reconnect_max_backoff_ms: 30000
```

Field descriptions:

| Field | Description |
|-------|-------------|
| `server_addr` | Platform gRPC service address (required) |
| `enroll_token` | Enrollment token, obtained when creating the Agent on the platform |
| `mtls.*` | Mutual TLS certificate paths (required in production) |
| `heartbeat_interval_seconds` | Heartbeat interval, default 15 seconds |
| `reconnect_*` | Backoff strategy for reconnection after disconnection |

Before enabling mTLS, ensure all three certificate files are deployed to the target paths. Once the connection is established, the Agent will automatically maintain an online status through heartbeats.

## Next Steps

- [Security Hardening Guide](security-hardening.md) -- Authentication, authorization, and network configuration
- [Operations Guide](operations-guide.md) -- Logging, monitoring, and troubleshooting
- [Gateway Tunnel Guide](gateway-tunnel-guide.md) -- Accessing internal network hosts through a jump server
- [Platform Integration Guide](platform-integration-guide.md) -- Complete platform integration process
