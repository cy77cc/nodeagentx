# OpsAgent 架构文档

本文档描述 OpsAgent 的整体架构、各子系统职责、数据流和关键设计决策。

## 1. 整体架构

OpsAgent 是一个运行在目标主机上的轻量级守护进程，通过 gRPC 与运维平台 (OpsPilot) 通信。Agent 采用模块化设计，各子系统通过接口解耦，支持独立的生命周期管理和热加载。

```
+===========================================================================+
|                            OpsPilot (平台)                                 |
|                     gRPC Server (双向流)                                    |
+===========================================================================+
         ^  gRPC bidirectional stream (mTLS)
         |  - 心跳 / 注册 / 指标 / 命令执行 / 隧道数据
         v
+===========================================================================+
|                           OpsAgent (本机)                                   |
+---------------------------------------------------------------------------+
|                                                                           |
|  +-------------------+      +------------------+      +----------------+  |
|  |  Collector         |      |  gRPC Client     |      |  HTTP Server   |  |
|  |  Pipeline          |      |                  |      |                |  |
|  |  ┌───────────┐     |      |  ┌────────────┐  |      | /health        |  |
|  |  │ Input     │     |      |  │ Heartbeat  │  |      | /metrics       |  |
|  |  │ (cpu,memory    |  +--->|  │ Reconnect  │  |      | /exec          |  |
|  |  │  disk,net...)  |      |  │ Cache      │  |      | /tasks         |  |
|  |  └─────┬─────┘     |      |  │ mTLS       │  |      | /config        |  |
|  |        v           |      |  └────────────┘  |      +--------+-------+  |
|  |  ┌───────────┐     |      +------------------+               |         |
|  |  │ Processor │     |                                         |         |
|  |  │ (regex,   │     |      +------------------+               |         |
|  |  │  delta,   │     |      |  Task Dispatcher |<--------------+         |
|  |  │  tagger)  │     |      +--------+---------+                          |
|  |  └─────┬─────┘     |               |                                    |
|  |        v           |       +-------+-------+                            |
|  |  ┌───────────┐     |       |       |       |                            |
|  |  │Aggregator│     |       v       v       v                            |
|  |  │(avg,sum, │     |  +-------+ +------+ +---------+                     |
|  |  │ minmax,  │     |  |Executor| |Sandbox| |Plugin  |                    |
|  |  │percentile│     |  |(whitelist| |(nsjail)| |Runtime |                   |
|  |  └─────┬─────┘   |  | exec)  | |exec) | |(Rust    |                    |
|  |        v          |  +-------+ +------+ | UDS)    |                    |
|  |  ┌───────────┐    |                     +----+----+                    |
|  |  │  Output   │    |                         |                          |
|  |  │(http,prom,│    |                  +------+-------+                  |
|  |  │ promrw)   │    |                  | Plugin       |                  |
|  |  └───────────┘    |                  | Gateway      |                  |
|  +-------------------+                  | (custom      |                  |
|                                         |  plugins)    |                  |
|  +-------------------+                  +--------------+                  |
|  |  Checker (5类)     |                                                    |
|  │  kernel           │      +------------------+                          |
|  │  filesystem       │      |  Gateway         |                          |
|  │  network          │      |  ┌────────────┐  |                          |
|  │  service          │      |  │ Tunnel     │  |                          |
|  │  container        │      |  │ Pool       │  |                          |
|  +-------------------+      |  ├────────────┤  |                          |
|                             |  │ Proxy      │  |                          |
|  +-------------------+      |  │ (SSH)      │  |                          |
|  |  Config Reloader  |      |  └────────────┘  |                          |
|  |  (watch + apply   |      +------------------+                          |
|  |   + rollback)     |                                                     |
|  +-------------------+      +------------------+                          |
|                             |  Audit Logger    |                          |
|                             |  (JSONL rotation)|                          |
|                             +------------------+                          |
+===========================================================================+
```

### 子系统总览

| 子系统 | 位置 | 职责 |
|--------|------|------|
| Collector Pipeline | `internal/collector/` | 系统指标采集、处理、聚合、上报 |
| Sandbox Executor | `internal/sandbox/` | 基于 nsjail 的安全命令沙箱执行 |
| Executor | `internal/executor/` | 白名单命令执行器（非沙箱模式） |
| gRPC Client | `internal/grpcclient/` | 与平台的双向 gRPC 通信 |
| Plugin Runtime | `internal/pluginruntime/` | Rust 进程管理、UDS IPC |
| Plugin Gateway | `internal/pluginruntime/gateway.go` | 自定义插件生命周期与路由 |
| Gateway | `internal/gateway/` | 隧道与代理（跳板机功能） |
| Checker | `internal/checker/` | 系统健康检查（5 大类） |
| HTTP Server | `internal/server/` | 本地 HTTP API（健康/指标/任务） |
| Config Reloader | `internal/config/reload.go` | 配置热加载与原子回滚 |
| Audit Logger | `internal/app/audit.go` | 结构化审计日志 |
| Log Collector | `internal/logcollector/` | 日志采集、解析与 OTLP 输出 |
| Tracing | `internal/tracing/` | 分布式追踪（OTLP 接收/导出、批处理） |
| Dashboard | `internal/dashboard/` | 本地嵌入式 HTML 仪表盘 |
| Alerting | `internal/alerting/` | 告警规则引擎与 Webhook 通知 |

## 2. 子系统职责

### 2.1 Collector Pipeline（指标采集管线）

**职责**: 以可配置的间隔采集系统指标，经过处理、聚合后输出到目标。

Collector 采用 Telegraf 风格的四阶段管线设计：

```
Input → Processor → Aggregator → Output
```

每个阶段通过插件注册表（`DefaultRegistry`）管理，使用 `init()` 自注册模式。管线支持运行时热重载：`Scheduler.Reload()` 方法会停止当前管线、重建所有插件实例、重新启动采集。

**关键接口**:

```go
// Input 采集原始指标
type Input interface {
    Init(config map[string]interface{}) error
    Gather(acc Accumulator) error
    SampleConfig() string
}

// Processor 在采集后转换指标
type Processor interface {
    Init(config map[string]interface{}) error
    Apply(metrics ...Metric) []Metric
}

// Aggregator 聚合一段时间内的指标
type Aggregator interface {
    Init(config map[string]interface{}) error
    Add(m Metric)
    Push(acc Accumulator)
    Reset()
}

// Output 将指标发送到外部目标
type Output interface {
    Init(config map[string]interface{}) error
    Write(metrics []Metric) error
}
```

**已注册插件**:

- Input: `cpu`, `memory`, `disk`, `net`, `process`, `load`, `diskio`, `temp`, `gpu`, `connections`
- Processor: `regex`, `delta`, `tagger`
- Aggregator: `avg`, `sum`, `minmax`, `percentile`
- Output: `http`, `prometheus`, `promrw`

**关键文件**: `internal/collector/scheduler.go`, `internal/collector/registry.go`, `internal/collector/inputs/`, `internal/collector/processors/`, `internal/collector/aggregators/`, `internal/collector/outputs/`

### 2.2 Sandbox Executor（沙箱执行器）

**职责**: 在 nsjail 沙箱中安全执行用户命令和脚本，提供命名空间隔离、资源限制和安全策略。

沙箱执行器通过 nsjail（Linux 命名空间隔离工具）在隔离环境中执行命令。支持命令与脚本两种模式，包含完整的安全策略引擎：

- 命令白名单/黑名单
- 关键字过滤（防止注入）
- Shell 注入检测
- 脚本大小限制
- 资源限制（内存、CPU、PID 数量）
- 输出流式读取与截断
- 网络隔离模式
- 审计日志记录

**关键接口**:

```go
// ExecRequest 沙箱执行请求
type ExecRequest struct {
    TaskID      string
    Command     string
    Args        []string
    Script      string
    Interpreter string
    Env         map[string]string
    Timeout     time.Duration
    SandboxCfg  *SandboxOverride // 每请求资源限制覆盖
}

// ExecResult 沙箱执行结果
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

**关键文件**: `internal/sandbox/executor.go`, `internal/sandbox/nsjail.go`, `internal/sandbox/policy.go`, `internal/sandbox/audit.go`, `internal/sandbox/output_streamer.go`, `internal/sandbox/network.go`

### 2.3 Executor（白名单执行器）

**职责**: 在非沙箱模式下执行白名单内的命令。提供基本的超时和输出限制。

**关键接口**:

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

**关键文件**: `internal/executor/executor.go`, `internal/executor/whitelist.go`

### 2.4 gRPC Client（gRPC 客户端）

**职责**: 维护与平台 (OpsPilot) 的双向 gRPC 流连接，处理心跳、断线重连、指标缓存和命令接收。

gRPC Client 是 Agent 与平台通信的核心组件，基于单个双向流（bidirectional stream）实现所有数据交换：

- **心跳**: 定期发送心跳保持连接活跃
- **重连**: 指数退避重连策略（初始 1s，最大 30s）
- **缓存**: 断线期间指标缓存到内存，重连后批量发送；支持持久化到磁盘
- **mTLS**: 支持双向 TLS 认证
- **接收**: `Receiver` 处理来自平台的命令（任务执行、隧道数据、健康检查等）

**关键接口**（`GRPCClient`，定义在 `internal/app/interfaces.go`）:

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
    // 隧道操作
    SendTunnelOpen(...) error
    SendTunnelData(...) error
    SendTunnelClose(...) error
    // 代理操作
    SendProxyRegister(...) error
    SendProxyResponse(...) error
    SendProxyMetrics(...) error
}
```

**关键文件**: `internal/grpcclient/client.go`, `internal/grpcclient/receiver.go`, `internal/grpcclient/cache.go`, `internal/grpcclient/proto/`

### 2.5 Plugin Runtime（插件运行时）

**职责**: 管理 Rust 进程的生命周期，通过 Unix Domain Socket (UDS) 进行 IPC 通信。

Plugin Runtime 管理一个独立的 Rust 进程，通过 UDS 上的 JSON-RPC 协议执行任务。Rust 进程作为 Agent 的子进程自动启动，支持启动超时、并发任务限制和分块传输。

**关键接口**（定义在 `internal/app/interfaces.go`）:

```go
type PluginRuntime interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ExecuteTask(ctx context.Context, req pluginruntime.TaskRequest) (*pluginruntime.TaskResponse, error)
    HealthStatus() health.Status
}
```

**关键文件**: `internal/pluginruntime/runtime.go`, `internal/pluginruntime/client.go`

### 2.6 Plugin Gateway（插件网关）

**职责**: 管理自定义插件的发现、生命周期、路由和热重载。

Plugin Gateway 扫描插件目录，自动发现并管理独立插件进程。每个插件通过 UDS 通信，支持任务类型路由、健康检查、自动重启和文件变更热重载。

**关键接口**（定义在 `internal/app/interfaces.go`）:

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

**关键文件**: `internal/pluginruntime/gateway.go`

### 2.7 Gateway（跳板机网关）

**职责**: 提供隧道和代理两种模式的跳板机功能，使平台能够访问内网主机。

Gateway 支持两种工作模式：

- **Tunnel 模式**: 平台通过 Agent 的 gRPC 隧道连接到内网主机，Agent 作为 TCP 代理转发数据。
- **Proxy 模式**: Agent 通过 SSH 直接连接内网主机执行命令，结果通过 gRPC 返回平台。
- **Auto 模式**: 自动选择最优模式。

Gateway 维护一个隧道连接池（`tunnel.Pool`），管理隧道的创建、数据传输和关闭。

**关键接口**（定义在 `internal/app/interfaces.go`）:

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

**内部接口**（定义在 `internal/gateway/gateway.go`）:

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

**关键文件**: `internal/gateway/gateway.go`, `internal/gateway/tunnel/pool.go`, `internal/gateway/proxy/`

### 2.8 Checker（健康检查器）

**职责**: 执行平台下发的系统健康检查任务，覆盖 5 大类检查项。

Checker 采用注册表模式，通过 `DefaultRegistry` 管理所有检查器。每类检查器实现 `Checker` 接口：

```go
type Checker interface {
    Type() string
    Check(ctx context.Context, params map[string]string) (*CheckResult, error)
}
```

`Executor` 接收平台下发的 `HealthCheckRequest`，逐项路由到对应的检查器，通过回调实时返回中间结果，最终返回汇总。

**5 大类检查器**:

| 类型 | 包 | 检查内容 |
|------|------|----------|
| kernel | `checker/kernel` | 内核参数、版本、模块 |
| filesystem | `checker/filesystem` | 磁盘空间、挂载点、权限 |
| network | `checker/network` | 网络连通性、端口、DNS |
| service | `checker/service` | 服务状态、端口监听 |
| container | `checker/container` | 容器运行状态、资源 |

**关键文件**: `internal/checker/executor.go`, `internal/checker/registry.go`, `internal/checker/kernel/`, `internal/checker/filesystem/`, `internal/checker/network/`, `internal/checker/service/`, `internal/checker/container/`

### 2.9 HTTP Server（本地 HTTP 服务）

**职责**: 提供本地 HTTP API，用于健康检查、Prometheus 指标导出、命令执行和任务管理。

HTTP Server 监听本地地址（默认 `127.0.0.1:18080`），提供以下端点：

| 端点 | 用途 |
|------|------|
| `GET /health` | 综合健康检查（汇总各子系统状态） |
| `GET /metrics` | Prometheus 格式指标导出 |
| `POST /exec` | 本地命令执行（白名单或沙箱） |
| `POST /tasks` | 任务提交（通过 Task Dispatcher 路由） |
| `POST /config` | 配置热更新 |

支持 Bearer Token 认证和 Prometheus 端点独立认证控制。

**关键接口**（定义在 `internal/app/interfaces.go`）:

```go
type HTTPServer interface {
    Start() error
    Shutdown(ctx context.Context) error
    SetLatestMetric(metric *collector.MetricPayload)
    LatestMetricExists() bool
}
```

**关键文件**: `internal/server/server.go`, `internal/server/handlers.go`

### 2.10 Config Reloader（配置热加载器）

**职责**: 监听配置变更，协调各子系统进行原子性的配置热更新和回滚。

Config Reloader 实现了两阶段提交式的配置更新：

1. 解析新配置 YAML
2. 计算变更集（ChangeSet）
3. 按顺序调用各子系统的 `CanReload()` 检查兼容性
4. 调用 `Apply()` 应用新配置
5. 如果任何子系统失败，对已更新的子系统调用 `Rollback()` 回滚

每个支持热加载的子系统实现 `Reloader` 接口：

```go
type Reloader interface {
    CanReload(cs *ChangeSet) bool
    Apply(newCfg *Config) error
    Rollback(oldCfg *Config) error
}
```

**关键文件**: `internal/config/reload.go`, `internal/config/reload_test.go`

### 2.11 Audit Logger（审计日志）

**职责**: 记录结构化的审计事件，支持日志轮转。

Audit Logger 以 JSON Lines 格式写入审计日志，使用 lumberjack 库实现日志轮转。记录的事件包括：

- 配置加载/变更
- gRPC 连接/断连
- 命令执行
- 沙箱执行
- 插件加载/卸载

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

**关键文件**: `internal/app/audit.go`

### 2.12 Log Collector（日志采集）

**职责**: 从多种来源采集日志，经过结构化解析后通过 OTLP 协议输出到后端。

Log Collector 支持三种输入源：文件 tail（按行追踪日志文件增长）、journald（读取 systemd 日志）和 syslog（监听 UDP/TCP syslog 端口）。采集到的原始日志经过 `logparse` 处理器进行正则匹配和字段提取，转换为结构化日志记录。结构化日志最终通过 OTLP gRPC/HTTP 协议导出到日志后端（如 OpenTelemetry Collector、Loki 等）。

**关键文件**: `internal/logcollector/`

### 2.13 Tracing（分布式追踪）

**职责**: 接收、处理和导出 OpenTelemetry 追踪数据，实现分布式链路追踪。

Tracing 子系统遵循 OpenTelemetry Collector 架构，由三个组件串联而成：OTLP Receiver 接收 gRPC（端口 4317）和 HTTP（端口 4318）两种协议的追踪数据；Batch Processor 对 span 进行批量聚合，通过可配置的超时和批次大小平衡延迟与吞吐；OTLP Exporter 将处理后的 trace 数据以 gRPC 协议发送到后端（如 Jaeger、Tempo）。追踪子系统默认关闭，启用后与其他子系统独立运行。

**关键文件**: `internal/tracing/`

### 2.14 Dashboard（本地仪表盘）

**职责**: 提供嵌入式 HTML 仪表盘，实时展示 Agent 状态和日志流。

Dashboard 是一个嵌入在 Agent 二进制中的轻量级 Web 界面，通过 Go 的 `embed` 包将前端资源编译进可执行文件，无需额外的静态文件部署。仪表盘通过 SSE（Server-Sent Events）实现日志的实时推送，浏览器建立 SSE 连接后即可接收 Agent 产生的结构化日志流。Dashboard 监听本地 HTTP 端口，仅用于开发调试和现场排障，不暴露到外网。

**关键文件**: `internal/dashboard/`

### 2.15 Alerting（智能告警）

**职责**: 基于可配置规则实时评估指标，触发告警并通过 Webhook 通知外部系统。

Alerting 引擎包含三个核心组件：规则管理器加载和热重载告警规则；评估引擎定期对采集到的指标执行规则表达式，判断是否满足告警条件；通知器通过 Webhook 将告警事件发送到外部系统（如企业微信、钉钉、PagerDuty）。告警状态机管理告警的生命周期（inactive → pending → firing → resolved），通过持续评估间隔和触发阈值避免告警抖动。

**关键文件**: `internal/alerting/`

## 3. 数据流

### 3.1 指标采集流

指标从 Input 插件采集，经过 Processor 转换和 Aggregator 聚合，最终通过 Output 插件和 gRPC 上报到平台。

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

**详细步骤**:

1. `Scheduler` 为每个 Input 启动独立 goroutine，按配置间隔调用 `Gather()`
2. `Input.Gather()` 将采集到的原始指标写入 `Accumulator`
3. `Accumulator` 将指标传递给 `Processor.Apply()` 进行转换（正则匹配、差值计算、标签添加等）
4. 处理后的指标被 `Aggregator.Add()` 收集，定期通过 `Push()` 输出聚合结果
5. 聚合后的指标通过 `Output.Write()` 发送到外部目标（HTTP、Prometheus remote write 等）
6. 同时，指标也通过 `gRPCClient.SendMetrics()` 上报到平台

### 3.2 命令执行流

平台通过 gRPC 下发执行命令，Agent 根据配置选择沙箱模式或白名单模式执行。

```
  Platform           gRPC Client        Task Dispatcher      Sandbox/Executor
     |                   |                    |                     |
     |-- ExecRequest --->|                    |                     |
     |                   |-- Dispatch() ----->|                     |
     |                   |                    |-- Route by type --->|
     |                   |                    |                     |
     |                   |                    |                     |
     |                   |                    |   [沙箱模式]          |
     |                   |                    |<-- nsjail.Execute --|
     |                   |                    |   (namespace隔离,    |
     |                   |                    |    cgroup限制,       |
     |                   |                    |    策略检查)          |
     |                   |                    |                     |
     |                   |                    |   [白名单模式]        |
     |                   |                    |<-- exec.Command ----|
     |                   |                    |   (白名单校验,       |
     |                   |                    |    超时控制)          |
     |                   |                    |                     |
     |                   |<-- ExecResult -----|                     |
     |<-- SendResult ----|                    |                     |
     |                   |                    |                     |
     |                   |  (流式输出)         |                     |
     |                   |<-- stdout/stderr --|                     |
     |<-- SendOutput ----|                    |                     |
```

**详细步骤**:

1. 平台通过 gRPC 双向流发送执行请求
2. `gRPCClient.Receiver` 接收请求，提交到 `Task Dispatcher`
3. `Dispatcher` 根据任务类型路由到对应执行器
4. 执行器运行命令：
   - **沙箱模式**: 通过 nsjail 创建隔离的命名空间，设置 cgroup 资源限制，检查安全策略，执行命令
   - **白名单模式**: 校验命令是否在白名单内，设置超时，直接执行
5. stdout/stderr 通过 `SendExecOutput()` 实时流式回传
6. 最终结果（退出码、耗时、是否超时）通过 `SendExecResult()` 回传

### 3.3 插件任务流

自定义插件任务通过 Plugin Gateway 路由到对应的插件进程。

```
  Platform/gRPC      Task Dispatcher      Plugin Gateway        Plugin Process
     |                    |                    |                     |
     |-- TaskRequest ---->|                    |                     |
     |                    |-- ExecuteTask() -->|                     |
     |                    |                    |-- Route by type --->|
     |                    |                    |   (查找注册的插件)     |
     |                    |                    |                     |
     |                    |                    |-- UDS JSON-RPC ---->|
     |                    |                    |   (Unix Domain Socket|
     |                    |                    |    通信)              |
     |                    |                    |                     |
     |                    |                    |<-- TaskResponse -----|
     |                    |<-- Response --------|                     |
     |<-- Result ---------|                    |                     |
```

**详细步骤**:

1. 平台或本地 HTTP API 提交插件任务请求
2. `Task Dispatcher` 将请求路由到 `Plugin Gateway`
3. `Plugin Gateway` 根据任务类型查找注册的插件（通过 `OnPluginLoaded` 回调注册的任务类型映射）
4. 通过 UDS（Unix Domain Socket）以 JSON-RPC 协议调用插件进程
5. 插件处理请求并返回结果
6. 结果通过 gRPC 或 HTTP 返回调用方

## 4. 设计决策

### 4.1 为什么选择 nsjail 作为沙箱

**选择**: 使用 Google 开源的 nsjail 作为命令沙箱执行环境。

**理由**:

- **命名空间隔离**: nsjail 原生支持 Linux namespaces（PID、mount、network、user、cgroup），提供进程级隔离而无需容器运行时
- **成熟稳定**: nsjail 由 Google 维护，在 Chrome 团队内部广泛用于代码执行隔离，经过大规模生产验证
- **无需内核模块**: 不依赖 seccomp-bpf 或 AppArmor 等需要额外配置的内核模块，开箱即用
- **细粒度控制**: 支持精确的资源限制（内存、CPU、PID 数量、文件大小）、网络隔离和文件系统挂载控制
- **轻量级**: 进程启动开销远低于容器方案（Docker/runc），适合高频的命令执行场景

**替代方案对比**:

| 方案 | 优势 | 劣势 |
|------|------|------|
| nsjail | 轻量、灵活、无需容器运行时 | 仅支持 Linux |
| Docker | 生态完善 | 启动开销大、需要守护进程 |
| gVisor | 强隔离 | 内核兼容性问题、性能损耗 |
| firejail | 简单 | 功能有限、安全性较弱 |
| seccomp | 内核级 | 配置复杂、调试困难 |

### 4.2 为什么选择 gRPC 双向流

**选择**: 使用单一 gRPC 双向流（bidirectional streaming）作为 Agent 与平台的唯一通信通道。

**理由**:

- **单一连接**: 所有通信复用一个 TCP 连接，减少连接管理开销和防火墙配置复杂度
- **服务端推送**: 平台可以随时向 Agent 推送命令（执行请求、隧道数据、健康检查），无需 Agent 轮询
- **高效传输**: gRPC 基于 HTTP/2，支持多路复用、头部压缩和二进制序列化（protobuf），比 REST API 更高效
- **类型安全**: protobuf 定义强类型接口，编译时检查，避免运行时序列化错误
- **流式传输**: 支持命令执行的 stdout/stderr 实时流式回传，无需等待命令结束
- **mTLS 支持**: gRPC 原生支持 mTLS，简化安全认证配置

### 4.3 为什么选择 Rust 实现插件运行时

**选择**: 使用 Rust 编写插件运行时进程，通过 UDS 与 Go 主进程通信。

**理由**:

- **性能**: Rust 的零成本抽象和无 GC 特性，确保插件执行的低延迟和可预测的内存行为
- **内存安全**: Rust 的所有权系统在编译期防止内存安全问题，降低插件运行时崩溃风险
- **UDS IPC**: Unix Domain Socket 提供高效的进程间通信，避免网络协议栈开销，适合高频任务调用
- **独立进程**: 插件运行时作为独立进程运行，崩溃不会影响 Agent 主进程，支持独立升级和重启
- **沙箱友好**: Rust 插件可以继承 Agent 的 nsjail 配置，在沙箱中运行，提供额外的安全层

### 4.4 为什么选择 Telegraf 风格的管线

**选择**: 采用 Input → Processor → Aggregator → Output 的四阶段管线设计。

**理由**:

- **经过验证的模式**: Telegraf 已在生产环境中被广泛使用（InfluxData 生态），该管线模式被证明是可靠和可扩展的
- **可组合性**: 每个阶段独立，可以自由组合不同的 Input、Processor、Aggregator 和 Output 插件
- **热重载**: 管线的每个阶段都可以独立重新初始化，支持运行时配置变更而无需重启 Agent
- **插件化**: 通过注册表模式（`DefaultRegistry`）和 `init()` 自注册，新插件只需实现接口并注册即可使用
- **关注点分离**: 每个阶段有明确的职责边界，便于独立测试和维护

### 4.5 其他设计决策

**接口隔离**: 所有子系统通过接口（定义在 `internal/app/interfaces.go`）与 Agent 核心交互，支持测试时注入 mock 实现。编译时通过类型断言检查接口满足性。

**编排器模式**: `Agent` 结构体作为编排器，负责组装所有子系统并管理其生命周期（启动、停止、健康检查）。`Task Dispatcher` 负责任务路由，将执行请求分发到对应的执行器。

**配置验证**: 配置加载时进行严格的验证（`Config.Validate()`），包括必填字段检查、值范围校验和条件依赖验证（如 sandbox 启用时必须配置 nsjail 路径）。启动前即发现配置错误，避免运行时故障。

**审计日志**: 所有关键操作（配置变更、连接状态、命令执行）记录结构化审计日志，支持安全合规和故障排查。日志通过 lumberjack 实现自动轮转和压缩。
