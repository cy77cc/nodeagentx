# OpsAgent HTTP API Reference

This document describes all HTTP API endpoints provided by OpsAgent.

## Common Response Format

All endpoints return a unified JSON response structure:

```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

| Field | Type | Description |
|-------|------|-------------|
| `success` | bool | Whether the request was successful |
| `data` | object/array | Data returned on success (omitted on failure) |
| `error` | string | Error message on failure (omitted on success) |

## Authentication

When server-side authentication is enabled (`auth.enabled = true`), all endpoints except `/healthz`, `/readyz`, and `/metrics` require a Bearer Token in the request header:

```
Authorization: Bearer <token>
```

## Rate Limiting

The server enables rate limiting by default: 10 requests per second per IP, with a burst limit of 20. When the limit is exceeded, `429 Too Many Requests` is returned.

---

## 1. GET /healthz -- Health Check

Returns the overall service health status and the status of each subsystem.

### Subsystems

| Subsystem | Type | Description |
|-----------|------|-------------|
| `grpc` | Core | gRPC client connection status |
| `scheduler` | Core | Scheduler running status |
| `plugin_runtime` | Optional | Plugin runtime status |
| `gateway` | Optional | Gateway connection status |

### Overall Status Determination Rules

- **healthy**: All subsystems are normal
- **degraded**: Optional subsystems are abnormal or unavailable
- **unhealthy**: Core subsystems are abnormal or unavailable

### Response

**200 OK**

```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "subsystems": {
      "grpc": {
        "status": "connected",
        "details": {
          "addr": "localhost:50051"
        }
      },
      "scheduler": {
        "status": "running"
      },
      "plugin_runtime": {
        "status": "unavailable"
      },
      "gateway": {
        "status": "unavailable"
      }
    }
  }
}
```

When authentication is enabled and the request carries a valid token, the response additionally includes version information:

```json
{
  "success": true,
  "data": {
    "status": "healthy",
    "subsystems": { ... },
    "version": "0.1.0",
    "git_commit": "abc1234",
    "uptime_seconds": 3600
  }
}
```

### Subsystem Status Values

| Status | Description |
|--------|-------------|
| `running` | Running normally |
| `connected` | Connected |
| `stopped` | Stopped |
| `error` | Error occurred |
| `disconnected` | Connection lost |
| `unavailable` | Component not initialized |

### curl Examples

```bash
curl http://localhost:8080/healthz

# With authentication (get version info)
curl -H "Authorization: Bearer <token>" http://localhost:8080/healthz
```

---

## 2. GET /readyz -- Readiness Probe

Used for readiness checks by orchestration platforms such as Kubernetes. Returns a ready status after at least one metric collection has been completed.

### Response

**200 OK** -- Ready

```json
{
  "success": true,
  "data": {
    "status": "ready"
  }
}
```

**503 Service Unavailable** -- Not Ready

```json
{
  "success": false,
  "error": "collector not ready"
}
```

### curl Examples

```bash
curl http://localhost:8080/readyz
```

---

## 3. POST /api/v1/exec -- Command Execution

Executes system commands from the allowlist. Request body limit is 1 MB.

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `command` | string | Yes | Name of the command to execute |
| `args` | string[] | No | List of command arguments |
| `timeout_seconds` | int | No | Timeout in seconds |

### Default Allowlist Commands

`uptime`, `df`, `free`, `whoami`, `hostname`, `ip`, `ss`

### Response

**200 OK**

```json
{
  "success": true,
  "data": {
    "exit_code": 0,
    "stdout": " 10:30:00 up 5 days,  3:22,  1 user,  load average: 0.10, 0.15, 0.12\n",
    "stderr": "",
    "duration_ms": 12
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `exit_code` | int | Process exit code |
| `stdout` | string | Standard output content |
| `stderr` | string | Standard error content |
| `duration_ms` | int64 | Execution duration in milliseconds |

**400 Bad Request** -- Invalid request body or command execution failed

```json
{
  "success": false,
  "error": "invalid request body"
}
```

or

```json
{
  "success": false,
  "error": "command execution failed"
}
```

**405 Method Not Allowed** -- Non-POST request

**429 Too Many Requests** -- Rate limit exceeded

### curl Examples

```bash
curl -X POST http://localhost:8080/api/v1/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "uptime", "args": [], "timeout_seconds": 5}'
```

```bash
curl -X POST http://localhost:8080/api/v1/exec \
  -H "Content-Type: application/json" \
  -d '{"command": "df", "args": ["-h"], "timeout_seconds": 10}'
```

---

## 4. POST /api/v1/tasks -- Task Execution

Dispatches and executes agent tasks. Request body limit is 1 MB.

### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `task_id` | string | Yes | Unique task identifier |
| `type` | string | Yes | Task type |
| `payload` | object | Yes | Task payload, structure varies by type |

### Supported Task Types

| Type | Description |
|------|-------------|
| `sandbox_exec` | Sandbox command execution |
| `exec_command` | Regular command execution |
| `health_check` | Health check |
| `collect_metrics` | Metric collection |
| `plugin_log_parse` | Log parsing plugin |
| `plugin_text_process` | Text processing plugin |
| `plugin_ebpf_collect` | eBPF collection plugin |
| `plugin_fs_scan` | Filesystem scan plugin |
| `plugin_conn_analyze` | Connection analysis plugin |
| `plugin_local_probe` | Local probe plugin |

### payload Common Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `timeout_seconds` | int | No | Timeout in seconds, default 15, maximum 300 |

### sandbox_exec payload

Sandbox execution supports two modes:

**Command Mode** (directly execute a command):

| Field | Type | Description |
|-------|------|-------------|
| `command` | string | Command name |
| `args` | string[] | Command arguments |

**Script Mode** (execute a script via an interpreter):

| Field | Type | Description |
|-------|------|-------------|
| `interpreter` | string | Interpreter path |
| `script` | string | Script content |

### Response

**200 OK** -- Response structure depends on the task type

```json
{
  "success": true,
  "data": {
    "task_id": "task-001",
    "status": "completed",
    "result": { ... }
  }
}
```

**400 Bad Request** -- Invalid request body or task dispatch failed

```json
{
  "success": false,
  "error": "invalid request body"
}
```

or

```json
{
  "success": false,
  "error": "task dispatch failed"
}
```

**405 Method Not Allowed** -- Non-POST request

### curl Examples

```bash
# Health check task
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "hc-001",
    "type": "health_check",
    "payload": {}
  }'
```

```bash
# Sandbox command execution
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "sb-001",
    "type": "sandbox_exec",
    "payload": {
      "command": "ls",
      "args": ["-la", "/tmp"],
      "timeout_seconds": 30
    }
  }'
```

```bash
# Sandbox script execution
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "sb-002",
    "type": "sandbox_exec",
    "payload": {
      "interpreter": "/bin/bash",
      "script": "echo hello && date",
      "timeout_seconds": 10
    }
  }'
```

---

## 5. GET /api/v1/metrics/latest -- Latest Metrics Snapshot

Retrieves the most recently collected host metric data.

### Response

**200 OK**

```json
{
  "success": true,
  "data": {
    "agent_id": "agent-001",
    "agent_name": "ops-agent-1",
    "collector": "system",
    "collected_at": "2026-05-25T10:30:00Z",
    "agent_started_at": "2026-05-25T08:00:00Z",
    "hostname": "web-server-01",
    "os": "linux",
    "platform": "ubuntu",
    "platform_version": "22.04",
    "kernel_version": "5.15.0-76-generic",
    "cpu_usage_percent": 23.5,
    "memory_usage_percent": 65.2,
    "disk_usage_percent": 42.8,
    "network_io": {
      "bytes_sent": 1048576,
      "bytes_recv": 2097152
    },
    "load_average": {
      "load1": 0.85,
      "load5": 0.72,
      "load15": 0.68
    }
  }
}
```

**404 Not Found** -- No metrics collected yet

```json
{
  "success": false,
  "error": "no metrics collected yet"
}
```

**405 Method Not Allowed** -- Non-GET request

### curl Examples

```bash
curl http://localhost:8080/api/v1/metrics/latest
```

---

## 6. GET /metrics -- Prometheus Metrics

Available when `prometheus.enabled = true` is configured. Returns metrics data in standard Prometheus text format, directly scrapable by a Prometheus server.

### Configuration

| Config Path | Type | Default | Description |
|-------------|------|---------|-------------|
| `prometheus.enabled` | bool | false | Whether to enable the Prometheus metrics endpoint |
| `prometheus.path` | string | `/metrics` | Metrics endpoint path |
| `prometheus.protect_with_auth` | bool | false | Whether authentication is required for access |

### Response

**200 OK** -- Returns Prometheus metrics text with `Content-Type: text/plain; version=0.0.4; charset=utf-8`.

### curl Examples

```bash
curl http://localhost:8080/metrics
```

---

## Status Code Summary

| Status Code | Meaning | Trigger Condition |
|-------------|---------|-------------------|
| 200 | OK | Request successful |
| 400 | Bad Request | Invalid request body, command execution failed, task dispatch failed |
| 401 | Unauthorized | Authentication failed (missing or invalid Bearer Token) |
| 404 | Not Found | Metrics data not collected |
| 405 | Method Not Allowed | Incorrect HTTP method |
| 429 | Too Many Requests | Rate limit exceeded (10 req/s) |
| 503 | Service Unavailable | Service not ready |
