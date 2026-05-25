# OpsAgent HTTP API 参考

本文档描述 OpsAgent 提供的所有 HTTP API 端点。

## 通用响应格式

所有端点均返回统一的 JSON 响应结构：

```json
{
  "success": true,
  "data": { ... },
  "error": ""
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `success` | bool | 请求是否成功 |
| `data` | object/array | 成功时返回的数据（失败时省略） |
| `error` | string | 失败时的错误信息（成功时省略） |

## 认证

当服务端启用认证（`auth.enabled = true`）时，除 `/healthz`、`/readyz` 和 `/metrics` 外的所有端点均需在请求头中携带 Bearer Token：

```
Authorization: Bearer <token>
```

## 限流

服务端默认启用速率限制：每 IP 10 请求/秒，突发上限 20。超出限制时返回 `429 Too Many Requests`。

---

## 1. GET /healthz -- 健康检查

返回服务整体健康状态及各子系统状态。

### 子系统

| 子系统 | 类型 | 说明 |
|--------|------|------|
| `grpc` | 核心 | gRPC 客户端连接状态 |
| `scheduler` | 核心 | 调度器运行状态 |
| `plugin_runtime` | 可选 | 插件运行时状态 |
| `gateway` | 可选 | 网关连接状态 |

### 整体状态判定规则

- **healthy**：所有子系统正常
- **degraded**：可选子系统异常或不可用
- **unhealthy**：核心子系统异常或不可用

### 响应

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

当认证启用且请求携带有效 Token 时，响应额外包含版本信息：

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

### 子系统状态值

| 状态 | 说明 |
|------|------|
| `running` | 正常运行 |
| `connected` | 已连接 |
| `stopped` | 已停止 |
| `error` | 出错 |
| `disconnected` | 连接断开 |
| `unavailable` | 组件未初始化 |

### curl 示例

```bash
curl http://localhost:8080/healthz

# 带认证（获取版本信息）
curl -H "Authorization: Bearer <token>" http://localhost:8080/healthz
```

---

## 2. GET /readyz -- 就绪探针

用于 Kubernetes 等编排平台的就绪检查。当至少完成一次指标采集后返回就绪状态。

### 响应

**200 OK** -- 已就绪

```json
{
  "success": true,
  "data": {
    "status": "ready"
  }
}
```

**503 Service Unavailable** -- 未就绪

```json
{
  "success": false,
  "error": "collector not ready"
}
```

### curl 示例

```bash
curl http://localhost:8080/readyz
```

---

## 3. POST /api/v1/exec -- 命令执行

执行白名单内的系统命令。请求体上限为 1 MB。

### 请求体

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `command` | string | 是 | 要执行的命令名称 |
| `args` | string[] | 否 | 命令参数列表 |
| `timeout_seconds` | int | 否 | 超时时间（秒） |

### 默认白名单命令

`uptime`、`df`、`free`、`whoami`、`hostname`、`ip`、`ss`

### 响应

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

| 字段 | 类型 | 说明 |
|------|------|------|
| `exit_code` | int | 进程退出码 |
| `stdout` | string | 标准输出内容 |
| `stderr` | string | 标准错误内容 |
| `duration_ms` | int64 | 执行耗时（毫秒） |

**400 Bad Request** -- 请求体无效或命令执行失败

```json
{
  "success": false,
  "error": "invalid request body"
}
```

或

```json
{
  "success": false,
  "error": "command execution failed"
}
```

**405 Method Not Allowed** -- 非 POST 请求

**429 Too Many Requests** -- 超出速率限制

### curl 示例

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

## 4. POST /api/v1/tasks -- 任务执行

分发并执行 Agent 任务。请求体上限为 1 MB。

### 请求体

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `task_id` | string | 是 | 任务唯一标识 |
| `type` | string | 是 | 任务类型 |
| `payload` | object | 是 | 任务载荷，结构因类型而异 |

### 支持的任务类型

| 类型 | 说明 |
|------|------|
| `sandbox_exec` | 沙箱命令执行 |
| `exec_command` | 普通命令执行 |
| `health_check` | 健康检查 |
| `collect_metrics` | 指标采集 |
| `plugin_log_parse` | 日志解析插件 |
| `plugin_text_process` | 文本处理插件 |
| `plugin_ebpf_collect` | eBPF 采集插件 |
| `plugin_fs_scan` | 文件系统扫描插件 |
| `plugin_conn_analyze` | 连接分析插件 |
| `plugin_local_probe` | 本地探测插件 |

### payload 通用字段

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `timeout_seconds` | int | 否 | 超时时间（秒），默认 15，最大 300 |

### sandbox_exec payload

沙箱执行支持两种模式：

**命令模式**（直接执行命令）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `command` | string | 命令名称 |
| `args` | string[] | 命令参数 |

**脚本模式**（通过解释器执行脚本）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `interpreter` | string | 解释器路径 |
| `script` | string | 脚本内容 |

### 响应

**200 OK** -- 响应结构取决于任务类型

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

**400 Bad Request** -- 请求体无效或任务分发失败

```json
{
  "success": false,
  "error": "invalid request body"
}
```

或

```json
{
  "success": false,
  "error": "task dispatch failed"
}
```

**405 Method Not Allowed** -- 非 POST 请求

### curl 示例

```bash
# 健康检查任务
curl -X POST http://localhost:8080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "task_id": "hc-001",
    "type": "health_check",
    "payload": {}
  }'
```

```bash
# 沙箱命令执行
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
# 沙箱脚本执行
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

## 5. GET /api/v1/metrics/latest -- 最新指标快照

获取最近一次采集的主机指标数据。

### 响应

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

**404 Not Found** -- 尚未采集到指标

```json
{
  "success": false,
  "error": "no metrics collected yet"
}
```

**405 Method Not Allowed** -- 非 GET 请求

### curl 示例

```bash
curl http://localhost:8080/api/v1/metrics/latest
```

---

## 6. GET /metrics -- Prometheus 指标

当配置 `prometheus.enabled = true` 时可用。返回标准 Prometheus 文本格式的指标数据，可直接被 Prometheus 服务器抓取。

### 配置项

| 配置路径 | 类型 | 默认值 | 说明 |
|----------|------|--------|------|
| `prometheus.enabled` | bool | false | 是否启用 Prometheus 指标端点 |
| `prometheus.path` | string | `/metrics` | 指标端点路径 |
| `prometheus.protect_with_auth` | bool | false | 是否需要认证访问 |

### 响应

**200 OK** -- 返回 `Content-Type: text/plain; version=0.0.4; charset=utf-8` 的 Prometheus 指标文本。

### curl 示例

```bash
curl http://localhost:8080/metrics
```

---

## 状态码汇总

| 状态码 | 含义 | 触发条件 |
|--------|------|----------|
| 200 | OK | 请求成功 |
| 400 | Bad Request | 请求体无效、命令执行失败、任务分发失败 |
| 401 | Unauthorized | 认证失败（未提供或无效的 Bearer Token） |
| 404 | Not Found | 指标数据未采集 |
| 405 | Method Not Allowed | HTTP 方法不正确 |
| 429 | Too Many Requests | 超出速率限制（10 req/s） |
| 503 | Service Unavailable | 服务未就绪 |
