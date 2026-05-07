# System Health Check Design Spec

**Date**: 2026-05-07
**Status**: Approved
**Scope**: Agent-side system health check, triggered by platform via gRPC

## Overview

Add a system health check feature to OpsAgent that allows the OpsPilot platform to trigger host-level health checks. The platform sends a list of check items (each referencing a typed checker with parameters), the agent executes them synchronously, and streams structured results back via gRPC.

This feature is independent from the existing `/healthz` endpoint (which checks agent subsystem status). System hardening is planned as a follow-up feature that shares check item definitions.

## gRPC Protocol Extension

### New Messages

**Platform-to-Agent** (added to `PlatformMessage.oneof`):
```protobuf
HealthCheckRequest health_check = 6;
```

**Agent-to-Platform** (added to `AgentMessage.oneof`):
```protobuf
HealthCheckResult health_check_result = 7;
```

### Message Definitions

```protobuf
message HealthCheckRequest {
  string request_id = 1;
  repeated CheckItem items = 2;
  int32 timeout_seconds = 3;
}

message CheckItem {
  string id = 1;
  string type = 2;
  string category = 3;
  string name = 4;
  string description = 5;
  bytes params = 6;
  CheckSeverity severity = 7;
}

enum CheckSeverity {
  SEVERITY_INFO = 0;
  SEVERITY_LOW = 1;
  SEVERITY_MEDIUM = 2;
  SEVERITY_HIGH = 3;
  SEVERITY_CRITICAL = 4;
}

message HealthCheckResult {
  string request_id = 1;
  repeated CheckResult results = 2;
  CheckSummary summary = 3;
  bool completed = 4;
}

message CheckResult {
  string item_id = 1;
  string type = 2;
  string name = 3;
  CheckStatus status = 4;
  string actual_value = 5;
  string expected_value = 6;
  string message = 7;
  string remediation = 8;
  CheckSeverity severity = 9;
  int64 duration_ms = 10;
}

enum CheckStatus {
  STATUS_PASS = 0;
  STATUS_FAIL = 1;
  STATUS_WARN = 2;
  STATUS_ERROR = 3;
  STATUS_SKIP = 4;
}

message CheckSummary {
  int32 total = 1;
  int32 pass = 2;
  int32 fail = 3;
  int32 warn = 4;
  int32 error = 5;
  int32 skip = 6;
  int64 total_duration_ms = 7;
}
```

### Streaming Behavior

- `HealthCheckResult.completed = false`: intermediate result (one per check item as it completes)
- `HealthCheckResult.completed = true`: final result with all results and summary
- Platform correlates requests/responses via `request_id`

## Checker Interface and Registry

### Checker Interface

```go
type Checker interface {
    Type() string
    Category() string
    Check(ctx context.Context, params json.RawMessage) (*CheckResult, error)
}
```

### Registry

```go
type Registry struct {
    mu       sync.RWMutex
    checkers map[string]Checker
}

var DefaultRegistry = &Registry{checkers: make(map[string]Checker)}

func Register(c Checker) { DefaultRegistry.Register(c) }
func (r *Registry) Register(c Checker) { ... }
func (r *Registry) Get(typ string) (Checker, bool) { ... }
func (r *Registry) Types() []string { ... }
```

Follows the same `init()` registration pattern as `collector.DefaultRegistry`.

### CheckResult (Go)

```go
type CheckResult struct {
    ItemID        string
    Type          string
    Name          string
    Status        CheckStatus
    ActualValue   string
    ExpectedValue string
    Message       string
    Remediation   string
    Severity      CheckSeverity
    Duration      time.Duration
}
```

## Executor

The `Executor` receives a `HealthCheckRequest`, iterates over check items, routes each to the corresponding `Checker`, and streams results back via a callback.

```go
type Executor struct {
    registry *Registry
    logger   zerolog.Logger
}

func (e *Executor) Execute(ctx context.Context, req *pb.HealthCheckRequest,
    callback func(*pb.HealthCheckResult)) error
```

Data flow:
```
Platform --HealthCheckRequest--> gRPC stream --> Receiver.Handle()
                                                      |
                                                      v
                                             HealthCheckHandler
                                                      |
                                                      v
                                             Executor.Execute()
                                                  |       |
                                             Checker A  Checker B
                                                  |       |
                                         callback(r1)  callback(r2)
                                                  |       |
                                        gRPC stream <----+
                                                      |
                                                      v
                                                   Platform
```

## gRPC Integration

### Receiver

Add `HealthCheckHandler` type and registration to `Receiver`:

```go
type HealthCheckHandler func(ctx context.Context, req *pb.HealthCheckRequest) error

func (r *Receiver) SetHealthCheckHandler(h HealthCheckHandler) { r.onHealthCheck = h }
```

Add case in `Handle()`:
```go
case *pb.PlatformMessage_HealthCheck:
    if r.onHealthCheck != nil {
        return r.onHealthCheck(ctx, p.HealthCheck)
    }
```

### Sender

Add to `sender.go`:
```go
func NewHealthCheckResultMessage(result *pb.HealthCheckResult) *pb.AgentMessage
```

### Capability Declaration

Agent declares supported checker types in `AgentRegistration.capabilities`:
```
["health_check", "checker:sysctl_check", "checker:file_perm_check", ...]
```

## Built-in Checkers

### Kernel & System Parameters (`kernel`)

| Type | Description | Implementation |
|------|-------------|----------------|
| `sysctl_check` | Kernel parameter value | Read `/proc/sys/` or `sysctl` |
| `kernel_version_check` | Kernel version range | `uname -r` + version comparison |
| `kernel_module_check` | Module loaded/not loaded | Read `/proc/modules` |
| `boot_param_check` | Boot parameters | Read `/proc/cmdline` |

### Filesystem Security (`filesystem`)

| Type | Description | Implementation |
|------|-------------|----------------|
| `file_perm_check` | File permissions and ownership | `os.Stat()` + `os.Lstat()` |
| `file_exist_check` | Sensitive file existence | `os.Stat()` |
| `dir_perm_check` | Directory permissions and sticky bit | `os.Stat()` |
| `mount_option_check` | Mount options (noexec, nosuid) | Parse `/proc/mounts` |

### Network Security (`network`)

| Type | Description | Implementation |
|------|-------------|----------------|
| `port_check` | Port listening state | Parse `/proc/net/tcp` or `ss` |
| `iptables_check` | Firewall rules | Parse `iptables -L -n` |
| `ssh_config_check` | SSH config items | Parse `/etc/ssh/sshd_config` |
| `network_param_check` | Network kernel params | Reuse `sysctl_check` |

### Services & Accounts (`service`)

| Type | Description | Implementation |
|------|-------------|----------------|
| `service_check` | systemd service status | `systemctl is-active` |
| `user_check` | User account state | Parse `/etc/passwd` + `/etc/shadow` |
| `cron_check` | Cron job audit | Read `/var/spool/cron/` + `/etc/cron.*` |
| `pam_check` | PAM config check | Parse `/etc/pam.d/` |

### Container Runtime (`container`)

| Type | Description | Implementation |
|------|-------------|----------------|
| `docker_check` | Docker config (cgroup driver, storage driver) | Parse `docker info` or `/etc/docker/daemon.json` |
| `containerd_check` | containerd config | Parse `/etc/containerd/config.toml` |
| `cgroup_check` | cgroup version and config | Read `/sys/fs/cgroup/` |
| `container_runtime_check` | Runtime availability | Check socket existence and connectivity |

### Params Schema Examples

```json
{"key": "net.ipv4.ip_forward", "expected": "0"}
{"path": "/etc/shadow", "expected_mode": "0640", "expected_owner": "root"}
{"name": "sshd", "expected_status": "active"}
{"port": 22, "expected_state": "listening"}
{"module": "dccp", "expected": "not_loaded"}
{"check": "cgroup_driver", "expected": "systemd"}
```

## Directory Structure

```
internal/checker/
├── registry.go
├── types.go
├── executor.go
├── kernel/
│   ├── sysctl.go
│   ├── kernel_version.go
│   ├── kernel_module.go
│   └── boot_param.go
├── filesystem/
│   ├── file_perm.go
│   ├── file_exist.go
│   ├── dir_perm.go
│   └── mount_option.go
├── network/
│   ├── port.go
│   ├── iptables.go
│   ├── ssh_config.go
│   └── network_param.go
├── service/
│   ├── service.go
│   ├── user.go
│   ├── cron.go
│   └── pam.go
└── container/
    ├── docker.go
    ├── containerd.go
    ├── cgroup.go
    └── runtime.go
```

## Configuration

```yaml
checker:
  enabled: true
  max_concurrent: 5
  default_timeout_seconds: 30
  disabled_checkers: []
```

## Security

- **Parameter validation**: Each checker validates params schema at entry
- **Path traversal prevention**: File-path checkers use `filepath.Clean()` + prefix whitelist
- **Timeout control**: Per-item timeout (from params or default) + overall request timeout
- **No privilege escalation**: Checkers run with agent process privileges (typically root)
- **Audit logging**: Each health check request logged (request_id, item_count, duration)

## Extensibility

- **Plugin checkers** (future): Load custom checkers via plugin gateway, declared in manifest `checker_types`
- **Hardening integration** (future): Checker `Remediation` field maps directly to hardening actions
- **Batch checks**: Already supported — one request contains multiple items, platform composes as needed

## Relationship to Existing Health System

| Feature | Existing `/healthz` | New System Health Check |
|---------|-------------------|------------------------|
| What it checks | Agent subsystems (gRPC, scheduler, plugins) | Host system configuration |
| Trigger | HTTP GET | gRPC message from platform |
| Scope | Agent health | OS/kernel/network/service/container config |
| Customizable | No | Yes — platform defines check items |
| Output | JSON (healthy/degraded/unhealthy) | Structured protobuf report |

The two systems are independent and do not interfere with each other.
