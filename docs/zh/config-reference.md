# OpsAgent 配置参考

本文档是 OpsAgent YAML 配置文件的完整参考。配置文件使用 YAML 格式，通过 `--config` 命令行参数指定路径。

---

## 目录

1. [agent — 代理标识与采集周期](#1-agent)
2. [server — 本地 API 服务](#2-server)
3. [executor — 命令执行限制](#3-executor)
4. [reporter — 数据上报](#4-reporter)
5. [auth — API 认证](#5-auth)
6. [prometheus — 指标导出](#6-prometheus)
7. [plugin — Rust 插件运行时](#7-plugin)
8. [grpc — gRPC 平台连接](#8-grpc)
9. [sandbox — nsjail 沙箱执行](#9-sandbox)
10. [collector — 指标采集管线](#10-collector)
11. [plugin_gateway — 自定义插件网关](#11-plugin_gateway)
12. [checker — 系统健康检查](#12-checker)
13. [gateway — 隧道/代理网关](#13-gateway)
14. [完整配置示例](#完整配置示例)

---

## 1. agent

控制代理身份标识和数据采集周期。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `agent.id` | string | — | 是 | 代理唯一标识符，用于在平台中区分不同代理实例 |
| `agent.name` | string | — | 是 | 代理人类可读名称，便于在管理界面中识别 |
| `agent.interval_seconds` | int | `10` | 是 | 数据采集间隔（秒），必须大于 0 |
| `agent.shutdown_timeout_seconds` | int | `30` | 否 | 优雅关闭等待超时（秒），超时后强制退出 |

### agent.audit_log

代理级别的审计日志配置。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `agent.audit_log.enabled` | bool | `false` | 否 | 是否启用审计日志 |
| `agent.audit_log.path` | string | `/var/log/opsagent/audit.jsonl` | 否 | 审计日志文件路径 |
| `agent.audit_log.max_size_mb` | int | `100` | 否 | 单个日志文件最大体积（MB），超过后轮转 |
| `agent.audit_log.max_backups` | int | `5` | 否 | 保留的历史日志文件数量 |

---

## 2. server

控制本地 HTTP API 服务设置。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `server.listen_addr` | string | `127.0.0.1:18080` | 是 | HTTP API 监听地址，格式为 `host:port` |

---

## 3. executor

控制命令执行的安全边界。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `executor.timeout_seconds` | int | `10` | 是 | 单次命令执行超时（秒），必须大于 0 |
| `executor.allowed_commands` | array[string] | — | 是 | 允许执行的命令白名单，不能为空，每项不能为空字符串 |
| `executor.max_output_bytes` | int | `65536` | 是 | 命令输出最大字节数（默认 64KB），必须大于 0 |

---

## 4. reporter

控制采集数据的上报方式。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `reporter.mode` | string | `stdout` | 是 | 上报模式，可选值：`stdout`（标准输出）、`http`（HTTP 推送） |
| `reporter.endpoint` | string | — | 条件必填 | HTTP 上报地址，当 `reporter.mode=http` 时必填 |
| `reporter.timeout_seconds` | int | `5` | 是 | 上报请求超时（秒），必须大于 0 |
| `reporter.retry_count` | int | `3` | 否 | 上报失败重试次数，必须大于等于 0 |
| `reporter.retry_interval_ms` | int | `500` | 否 | 重试间隔（毫秒），必须大于等于 0 |

---

## 5. auth

控制 API 接口的认证机制。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `auth.enabled` | bool | `true` | 否 | 是否启用 Bearer Token 认证 |
| `auth.bearer_token` | string | — | 条件必填 | 认证令牌，当 `auth.enabled=true` 时必填，长度不少于 32 字符 |

> **安全提示**：生产环境务必启用认证并设置强随机令牌。令牌不足 32 字符时启动将报错。

---

## 6. prometheus

控制 Prometheus 指标导出端点。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `prometheus.enabled` | bool | `true` | 否 | 是否启用 Prometheus 指标端点 |
| `prometheus.path` | string | `/metrics` | 条件必填 | 指标端点 URL 路径，启用时必须以 `/` 开头 |
| `prometheus.protect_with_auth` | bool | `false` | 否 | 是否复用 `auth` 配置保护指标端点 |

---

## 7. plugin

控制 Rust 插件运行时集成。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `plugin.enabled` | bool | `false` | 否 | 是否启用 Rust 插件运行时 |
| `plugin.runtime_path` | string | `./rust-runtime/target/release/github.com/cy77cc/opsagent-rust-runtime` | 条件必填 | Rust 运行时二进制路径，`auto_start=true` 时必填 |
| `plugin.socket_path` | string | `/tmp/github.com/cy77cc/opsagent/plugin.sock` | 条件必填 | Unix Domain Socket 路径，启用时必填 |
| `plugin.auto_start` | bool | `true` | 否 | 是否在代理启动时自动启动运行时进程 |
| `plugin.startup_timeout_seconds` | int | `5` | 是 | 运行时启动超时（秒），必须大于 0 |
| `plugin.request_timeout_seconds` | int | `30` | 是 | 单次插件请求超时（秒），必须大于 0 |
| `plugin.max_concurrent_tasks` | int | `4` | 是 | 最大并发插件任务数，必须大于 0 |
| `plugin.max_result_bytes` | int | `8388608` | 是 | 插件返回结果最大字节数（默认 8MB），必须大于 0 |
| `plugin.chunk_size_bytes` | int | `262144` | 是 | 大结果分块传输的块大小（默认 256KB），必须大于 0 |
| `plugin.sandbox_profile` | string | `strict` | 是 | 插件沙箱策略，如 `strict`、`relaxed` 等 |

---

## 8. grpc

控制与管理平台的 gRPC 连接。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `grpc.server_addr` | string | — | 是 | gRPC 服务端地址，格式为 `host:port` |
| `grpc.enroll_token` | string | — | 否 | 代理注册令牌，首次入网时使用 |
| `grpc.heartbeat_interval_seconds` | int | `15` | 是 | 心跳发送间隔（秒），必须大于 0 |
| `grpc.reconnect_initial_backoff_ms` | int | `1000` | 是 | 断线重连初始退避时间（毫秒），必须大于 0 |
| `grpc.reconnect_max_backoff_ms` | int | `30000` | 是 | 断线重连最大退避时间（毫秒），必须大于 0 |
| `grpc.cache_persist_path` | string | `""` | 否 | 本地缓存持久化路径，为空则仅内存缓存 |

### grpc.mtls

双向 TLS 证书配置。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `grpc.mtls.cert_file` | string | `""` | 否 | 客户端证书文件路径 |
| `grpc.mtls.key_file` | string | `""` | 否 | 客户端私钥文件路径 |
| `grpc.mtls.ca_file` | string | `""` | 否 | CA 证书文件路径，用于验证服务端 |

---

## 9. sandbox

控制基于 nsjail 的命令沙箱执行环境。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `sandbox.enabled` | bool | `false` | 否 | 是否启用沙箱执行 |
| `sandbox.nsjail_path` | string | — | 条件必填 | nsjail 二进制文件路径，启用时必填 |
| `sandbox.base_workdir` | string | — | 条件必填 | 沙箱工作目录根路径，启用时必填 |
| `sandbox.default_timeout_seconds` | int | `30` | 是 | 沙箱内命令默认超时（秒），必须大于 0 |
| `sandbox.max_concurrent_tasks` | int | `4` | 是 | 最大并发沙箱任务数，必须大于 0 |
| `sandbox.cgroup_base_path` | string | — | 条件必填 | cgroup 挂载基路径，启用时必填 |
| `sandbox.audit_log_path` | string | — | 条件必填 | 沙箱审计日志路径，启用时必填 |
| `sandbox.allow_unsandboxed_fallback` | bool | `false` | 否 | 当沙箱不可用时是否回退到非沙箱执行 |

### sandbox.policy

沙箱安全策略。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `sandbox.policy.allowed_commands` | array[string] | — | 条件必填 | 沙箱内允许执行的命令列表，启用时不能为空 |
| `sandbox.policy.blocked_commands` | array[string] | `[]` | 否 | 明确禁止执行的命令列表 |
| `sandbox.policy.blocked_keywords` | array[string] | `[]` | 否 | 命令中禁止出现的关键字（如 `"rm -rf /"`） |
| `sandbox.policy.allowed_interpreters` | array[string] | `[]` | 否 | 允许使用的脚本解释器（如 `bash`、`python3`） |
| `sandbox.policy.script_max_bytes` | int | `65536` | 是 | 脚本内容最大字节数（默认 64KB），必须大于 0 |
| `sandbox.policy.shell_injection_check` | bool | `true` | 否 | 是否启用 Shell 注入检测 |

---

## 10. collector

定义指标采集管线，由输入、处理器、聚合器、输出四个阶段组成。每个阶段是一个插件实例列表。

### 通用字段

每个插件实例包含：

| 字段 | 类型 | 说明 |
|------|------|------|
| `type` | string | 插件类型标识 |
| `config` | object | 该插件类型的特定配置参数 |

### collector.inputs

数据输入源列表。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `collector.inputs` | array[PluginInstanceConfig] | — | 否 | 输入插件列表 |

常用输入类型：`cpu`、`memory`、`disk`、`net`、`load`、`diskio`、`temp`、`gpu`、`connections`。

### collector.processors

数据处理器列表。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `collector.processors` | array[PluginInstanceConfig] | `[]` | 否 | 处理器插件列表，用于数据变换（如 delta/rate 计算） |

### collector.aggregators

数据聚合器列表。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `collector.aggregators` | array[PluginInstanceConfig] | `[]` | 否 | 聚合器插件列表，用于统计数据（如 min/max、percentile） |

### collector.outputs

数据输出目标列表。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `collector.outputs` | array[PluginInstanceConfig] | `[]` | 否 | 输出插件列表，定义数据去向 |

---

## 11. plugin_gateway

管理自定义插件的发现、启动和生命周期。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `plugin_gateway.enabled` | bool | `false` | 否 | 是否启用插件网关 |
| `plugin_gateway.plugins_dir` | string | `/etc/opsagent/plugins` | 条件必填 | 插件目录路径，启用时必填 |
| `plugin_gateway.startup_timeout_seconds` | int | `10` | 是 | 插件启动超时（秒），必须大于 0 |
| `plugin_gateway.health_check_interval_seconds` | int | `30` | 是 | 健康检查间隔（秒），必须大于 0 |
| `plugin_gateway.max_restarts` | int | `3` | 否 | 插件最大重启次数，必须大于等于 0 |
| `plugin_gateway.restart_backoff_seconds` | int | `5` | 否 | 重启退避时间（秒） |
| `plugin_gateway.file_watch_debounce_seconds` | int | `2` | 否 | 文件变更监听防抖时间（秒） |
| `plugin_gateway.plugin_configs` | object | — | 否 | 各插件的独立配置，键为插件名，值为配置键值对 |

---

## 12. checker

控制系统健康检查子系统。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `checker.enabled` | bool | `true` | 否 | 是否启用健康检查 |
| `checker.max_concurrent` | int | `5` | 是 | 最大并发检查数，启用时必须大于 0 |
| `checker.default_timeout_seconds` | int | `30` | 是 | 单项检查默认超时（秒），启用时必须大于 0 |
| `checker.disabled_checkers` | array[string] | `[]` | 否 | 禁用的检查器名称列表 |

---

## 13. gateway

控制隧道/代理网关子系统，用于跳板机场景。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `gateway.enabled` | bool | `false` | 否 | 是否启用网关 |
| `gateway.listen_addr` | string | `:18081` | 条件必填 | 网关监听地址，启用时必填 |
| `gateway.max_tunnels` | int | `100` | 是 | 最大隧道数量，启用时必须大于 0 |
| `gateway.tunnel_timeout_seconds` | int | `30` | 是 | 隧道建立超时（秒），启用时必须大于 0 |
| `gateway.idle_timeout_seconds` | int | `300` | 是 | 空闲隧道回收超时（秒），启用时必须大于 0 |
| `gateway.hosts` | array[GatewayHostConfig] | `[]` | 否 | 内网主机列表 |

### gateway.hosts[]

定义网关背后的内网主机。

| YAML 路径 | 类型 | 默认值 | 必填 | 说明 |
|-----------|------|--------|------|------|
| `gateway.hosts[].id` | string | — | 是 | 主机唯一标识 |
| `gateway.hosts[].addr` | string | — | 是 | 主机地址，格式为 `host:port` |
| `gateway.hosts[].mode` | string | — | 是 | 连接模式，可选值：`tunnel`（隧道）、`proxy`（代理）、`auto`（自动选择） |
| `gateway.hosts[].ssh.user` | string | — | 条件必填 | SSH 用户名，`mode=proxy` 或 `mode=auto` 时必填 |
| `gateway.hosts[].ssh.password` | string | — | 否 | SSH 密码（建议使用密钥认证） |
| `gateway.hosts[].ssh.key_file` | string | — | 否 | SSH 私钥文件路径 |
| `gateway.hosts[].ssh.port` | int | — | 条件必填 | SSH 端口，`mode=proxy` 或 `mode=auto` 时必填且大于 0 |

---

## 完整配置示例

```yaml
# ============================================================
# OpsAgent 完整配置示例
# ============================================================

# 1. 代理标识与采集周期
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

# 2. 本地 API 服务
server:
  listen_addr: "127.0.0.1:18080"

# 3. 命令执行限制
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

# 4. 数据上报
reporter:
  mode: "stdout"          # stdout | http
  endpoint: ""
  timeout_seconds: 5
  retry_count: 3
  retry_interval_ms: 500

# 5. API 认证
auth:
  enabled: false
  bearer_token: ""        # 启用时须设置 32+ 字符的令牌

# 6. Prometheus 指标导出
prometheus:
  enabled: true
  path: "/metrics"
  protect_with_auth: false

# 7. Rust 插件运行时
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

# 8. gRPC 平台连接
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

# 9. nsjail 沙箱执行
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

# 10. 指标采集管线
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

# 11. 自定义插件网关
plugin_gateway:
  enabled: false
  plugins_dir: "/etc/opsagent/plugins"
  startup_timeout_seconds: 10
  health_check_interval_seconds: 30
  max_restarts: 3
  restart_backoff_seconds: 5
  file_watch_debounce_seconds: 2
  plugin_configs: {}

# 12. 系统健康检查
checker:
  enabled: true
  max_concurrent: 5
  default_timeout_seconds: 30
  disabled_checkers: []

# 13. 隧道/代理网关
gateway:
  enabled: false
  listen_addr: ":18081"
  max_tunnels: 100
  tunnel_timeout_seconds: 30
  idle_timeout_seconds: 300
  hosts: []
  # 示例主机配置：
  # hosts:
  #   - id: "vm-web-01"
  #     addr: "192.168.122.100:22"
  #     mode: "auto"
  #     ssh:
  #       user: "root"
  #       key_file: "/etc/opsagent/keys/id_rsa"
  #       port: 22
```
