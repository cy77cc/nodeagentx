# Spec: 可观测性增强

> 日期: 2026-05-27
> 状态: Approved
> 作者: AI Assistant + User

## Context

OpsAgent 已完整实现指标采集管线（10 Input + 3 Processor + 4 Aggregator + 3 Output）、沙箱执行、gRPC 双向流、插件系统、Gateway 跳板机、健康检查等核心功能。当前缺少日志采集、分布式追踪、本地调试界面和本地告警能力。本 spec 定义这四个可观测性增强功能的设计。

## 目标

1. 日志采集转发：支持文件尾随、journald、syslog 三种日志源，边缘解析后通过 OTLP 导出
2. 分布式追踪：作为 OTLP Receiver/Exporter 接收本地应用 span，转发到平台
3. 本地 Dashboard：嵌入式 Web UI，用于快速调试和状态查看
4. 智能告警：本地规则引擎，支持阈值告警和 webhook/平台通知

## 依赖

- 现有 Collector Pipeline 架构（Input/Processor/Aggregator/Output 接口）
- 现有 HTTP Server（`internal/server/`）
- 现有 gRPC Client（`internal/grpcclient/`）
- 现有配置热重载（`internal/config/reload.go`）

## 设计

### 1. 日志采集转发

#### 1.1 架构

新增三个 Input 插件和一个 Processor 插件，复用现有 Collector Pipeline：

```
journald/tail/syslog → Accumulator → logparse → Aggregator → otlp/http output
                                                      → gRPCClient.SendMetrics()
```

#### 1.2 组件

| 组件 | 包路径 | 职责 |
|------|--------|------|
| `inputs/tail` | `internal/collector/inputs/tail/` | 文件尾随采集，游标持久化，inotify/poll 模式 |
| `inputs/journald` | `internal/collector/inputs/journald/` | systemd journal 读取，unit/优先级过滤 |
| `inputs/syslog` | `internal/collector/inputs/syslog/` | TCP/UDP Syslog 接收器，RFC 5424/3164 解析 |
| `processors/logparse` | `internal/collector/processors/logparse/` | Grok/regex/JSON 结构化解析 |
| `outputs/otlp` | `internal/collector/outputs/otlp/` | OTLP Logs 协议导出，gRPC + HTTP |

#### 1.3 tail Input

```go
// internal/collector/inputs/tail/tail.go
type TailInput struct {
    Files            []string `toml:"files"`              // glob 模式
    WatchMethod      string   `toml:"watch_method"`       // inotify | poll
    FromBeginning    bool     `toml:"from_beginning"`
    CursorPersistPath string  `toml:"cursor_persist_path"`
    MaxLineBytes     int      `toml:"max_line_bytes"`
    GrokPatterns     []string `toml:"grok_patterns"`
}
```

**游标持久化**：每个文件记录 `{path, offset, inode}` 到 JSON 文件，重启后断点续传。

**关键实现**：
- 使用 `fsnotify` 监听文件变更（已有依赖）
- 使用 `bufio.Scanner` 按行读取，`MaxLineBytes` 截断超长行
- 文件轮转检测：inode 变更时重新打开文件

#### 1.4 journald Input

```go
// internal/collector/inputs/journald/journald.go
type JournaldInput struct {
    Units            []string `toml:"units"`              // 过滤的 systemd unit
    Priority         string   `toml:"priority"`           // emerg..debug
    CursorPersistPath string  `toml:"cursor_persist_path"`
    SinceCursor      string   `toml:"since_cursor"`
}
```

**依赖**：`github.com/coreos/go-systemd/v22/sdjournal`

**字段映射**：
| journal 字段 | Metric field |
|-------------|-------------|
| `MESSAGE` | `message` |
| `_PID` | `pid` |
| `_COMM` | `command` |
| `_SYSTEMD_UNIT` | `unit` |
| `PRIORITY` | `priority` |
| `__REALTIME_TIMESTAMP` | `timestamp` |

#### 1.5 syslog Input

```go
// internal/collector/inputs/syslog/syslog.go
type SyslogInput struct {
    ListenAddr string `toml:"listen_addr"`    // "0.0.0.0:514"
    Protocol   string `toml:"protocol"`       // tcp | udp
    TLS        *TLSConfig `toml:"tls"`
    MaxConnections int   `toml:"max_connections"`
}
```

**解析器**：支持 RFC 5424（structured）和 RFC 3164（BSD）两种格式，自动检测。

#### 1.6 logparse Processor

```go
// internal/collector/processors/logparse/logparse.go
type LogParseProcessor struct {
    Rules []ParseRule `toml:"rules"`
}

type ParseRule struct {
    Field        string `toml:"field"`         // 要解析的 field 名
    Parser       string `toml:"parser"`        // grok | regex | json
    GrokPattern  string `toml:"grok_pattern"`
    RegexPattern string `toml:"regex_pattern"`
    JSONPaths    []string `toml:"json_paths"`  // JSON 提取路径
}
```

**Grok 实现**：内置常用 pattern（`%{IPORHOST}`、`%{HTTPD_COMBINEDLOG}`、`%{TIMESTAMP_ISO8601}`、`%{LOGLEVEL}`、`%{NUMBER}` 等，兼容 Telegraf/grok 格式），编译为正则表达式并缓存。自定义 pattern 通过 `patterns` 字段定义：

```yaml
processors:
  - type: logparse
    config:
      patterns:
        CUSTOM_LOG: '%{IPORHOST:client_ip} %{DATA:user} \[%{TIMESTAMP_ISO8601:ts}\]'
      rules:
        - field: "message"
          parser: "grok"
          grok_pattern: '%{CUSTOM_LOG}'
```

#### 1.7 otlp Output

```go
// internal/collector/outputs/otlp/otlp.go
type OTLPOutput struct {
    Endpoint    string     `toml:"endpoint"`
    Protocol    string     `toml:"protocol"`     // grpc | http
    MTLS        *MTLSConfig `toml:"mtls"`
    Headers     map[string]string `toml:"headers"`
    Compression string     `toml:"compression"`  // gzip | none
    BatchSize   int        `toml:"batch_size"`
    Timeout     int        `toml:"timeout_seconds"`
}
```

**依赖**：`go.opentelemetry.io/otel/exporters/otlp/otlplog`

#### 1.8 配置

```yaml
collector:
  inputs:
    - type: journald
      config:
        units: ["nginx", "docker", "sshd"]
        priority: "info"
        cursor_persist_path: "/var/lib/opsagent/journal.cursor"
    - type: tail
      config:
        files: ["/var/log/nginx/*.log", "/var/log/syslog"]
        watch_method: "inotify"
        from_beginning: false
        cursor_persist_path: "/var/lib/opsagent/tail.cursor"
        max_line_bytes: 65536
    - type: syslog
      config:
        listen_addr: "0.0.0.0:514"
        protocol: "tcp"
        tls:
          cert_file: ""
          key_file: ""
  processors:
    - type: logparse
      config:
        rules:
          - field: "message"
            parser: "grok"
            grok_pattern: '%{IPORHOST:client_ip} ...'
  outputs:
    - type: otlp
      config:
        endpoint: "platform.example.com:4317"
        protocol: "grpc"
```

### 2. 分布式追踪

#### 2.1 架构

Agent 作为 OTLP Receiver/Exporter，不做 trace 后端。

```
本地应用 → OTLP gRPC/HTTP → Agent Receiver → Processor → Exporter → 平台/Jaeger/Tempo
```

#### 2.2 组件

| 组件 | 包路径 | 职责 |
|------|--------|------|
| `internal/tracing/receiver.go` | OTLP gRPC/HTTP 端点 | 接收本地应用 span |
| `internal/tracing/processor.go` | 批处理 + 属性富化 | 添加 host metadata、采样 |
| `internal/tracing/exporter.go` | OTLP 转发 | 发送到平台或后端 |

#### 2.3 配置

```yaml
tracing:
  enabled: false
  receiver:
    grpc_addr: "0.0.0.0:4317"
    http_addr: "0.0.0.0:4318"
  processor:
    batch_timeout_ms: 5000
    max_batch_size: 512
    tail_sampling:
      enabled: false
      policies: []
  exporter:
    endpoint: "platform.example.com:4317"
    protocol: "grpc"
    mtls: {}
```

#### 2.4 接口集成

在 `Agent` 中新增 `TracingReceiver` 子系统：

```go
// internal/app/interfaces.go
type TracingReceiver interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    HealthStatus() health.Status
}
```

启动顺序：在 HTTP Server 之后启动 TracingReceiver。

### 3. 本地 Dashboard

#### 3.1 架构

嵌入式单页 HTML，Go `embed.FS`，无 JS 框架，自动刷新。

#### 3.2 新增端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/ui/` | GET | 嵌入式 HTML dashboard |
| `/api/v1/config` | GET | 当前运行配置（secrets 脱敏） |
| `/api/v1/plugins` | GET | 已加载插件列表及状态 |
| `/api/v1/logs` | GET | SSE 实时日志流 |
| `/api/v1/health/detailed` | GET | 各子系统详细健康状态 |

#### 3.3 Dashboard 页面

- **系统概览**：CPU/内存/磁盘/网络实时图表（SVG 绘制，无外部依赖）
- **子系统状态**：gRPC、Scheduler、Plugin Runtime、Gateway 状态卡片
- **最近指标**：最后一次采集的指标表格
- **日志查看**：实时 SSE 日志流，支持级别过滤（debug/info/warn/error）
- **配置查看**：当前配置树形展示

#### 3.4 实现

```go
// internal/server/ui.go
//go:embed ui/*
var uiFS embed.FS

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
    fs := http.FS(uiFS)
    http.StripPrefix("/ui/", http.FileServer(fs)).ServeHTTP(w, r)
}

func (s *Server) handleLogsSSE(w http.ResponseWriter, r *http.Request) {
    // SSE 实现：ring buffer 存储最近 1000 条日志
    // zerolog Hook 将日志写入 ring buffer
    // SSE handler 从 ring buffer 读取并推送
}
```

### 4. 智能告警

#### 4.1 架构

本地规则引擎，每次指标采集后评估规则，触发 webhook/平台通知。

#### 4.2 组件

| 组件 | 包路径 | 职责 |
|------|--------|------|
| `internal/alerting/engine.go` | 规则评估引擎 | 每次采集后评估所有规则 |
| `internal/alerting/rules.go` | YAML 规则定义 | 支持阈值、持续时间、组合条件 |
| `internal/alerting/notifier.go` | 通知器 | Webhook/日志/平台通知 |

#### 4.3 规则格式

```yaml
alerting:
  enabled: false
  rules:
    - name: "high_cpu"
      condition:
        metric: "cpu_usage_percent"
        operator: ">"          # >, <, >=, <=, ==, !=
        threshold: 95
        for: "5m"              # 持续时间
      severity: "critical"     # info, warning, critical
      notify:
        - type: "webhook"
          url: "https://hooks.example.com/alert"
          headers:
            Authorization: "Bearer xxx"
        - type: "platform"     # 通过 gRPC 转发到平台
    - name: "disk_low"
      condition:
        metric: "disk_free_percent"
        operator: "<"
        threshold: 10
        for: "10m"
      severity: "warning"
      notify:
        - type: "platform"
```

#### 4.4 状态机

```
OK → PENDING (条件满足，开始计时) → FIRING (持续时间到达) → OK (条件不满足)
```

- **PENDING**：条件首次满足，开始计时（`for` 字段）
- **FIRING**：持续时间到达，触发通知
- **OK**：条件不再满足，发送恢复通知

#### 4.5 集成点

- `Agent.eventLoop()` 中，每次从 pipeline 收到指标后调用 `engine.Evaluate(metrics)`
- 告警状态变更时调用 `notifier.Notify(alert, state)`
- 平台通知通过 `gRPCClient` 发送新的 `AlertState` 消息

#### 4.6 Proto 扩展

```protobuf
message AlertState {
    string agent_id = 1;
    string rule_name = 2;
    string severity = 3;
    string state = 4;           // ok, pending, firing
    double current_value = 5;
    double threshold = 6;
    int64 triggered_at = 7;
    int64 resolved_at = 8;
    string message = 9;
}
```

## 关键文件

| 文件 | 操作 |
|------|------|
| `internal/collector/inputs/tail/` | **新建** — 文件尾随 Input |
| `internal/collector/inputs/journald/` | **新建** — journald Input |
| `internal/collector/inputs/syslog/` | **新建** — syslog Input |
| `internal/collector/processors/logparse/` | **新建** — 日志解析 Processor |
| `internal/collector/outputs/otlp/` | **新建** — OTLP Output |
| `internal/tracing/` | **新建** — 分布式追踪子系统 |
| `internal/server/ui/` | **新建** — 嵌入式 Dashboard HTML |
| `internal/server/ui.go` | **新建** — UI 端点 handler |
| `internal/alerting/` | **新建** — 智能告警引擎 |
| `internal/app/interfaces.go` | **修改** — 新增 TracingReceiver 接口 |
| `internal/app/agent.go` | **修改** — 集成告警引擎和追踪 |
| `internal/config/config.go` | **修改** — 新增 tracing/alerting 配置段 |
| `proto/agent.proto` | **修改** — 新增 AlertState 消息 |

## 测试要求

- tail Input：文件追加、轮转、断点续传、glob 匹配
- journald Input：unit 过滤、优先级过滤、游标续传
- syslog Input：TCP/UDP 接收、RFC 5424/3164 解析、TLS
- logparse Processor：grok/regex/JSON 解析、性能基准
- otlp Output：gRPC/HTTP 导出、批量、重试
- Dashboard：端点可访问、SSE 推送、secrets 脱敏
- 告警引擎：阈值评估、持续时间状态机、webhook 通知、恢复通知

## 验证方式

```bash
# 单元测试
go test -race ./internal/collector/inputs/tail/...
go test -race ./internal/collector/inputs/journald/...
go test -race ./internal/collector/inputs/syslog/...
go test -race ./internal/collector/processors/logparse/...
go test -race ./internal/collector/outputs/otlp/...
go test -race ./internal/tracing/...
go test -race ./internal/alerting/...
go test -race ./internal/server/ -run TestUI

# 集成测试
go test -race -tags=integration ./internal/integration/ -run TestLogCollection
go test -race -tags=integration ./internal/integration/ -run TestTracing
go test -race -tags=integration ./internal/integration/ -run TestAlerting

# 手动验证
curl http://127.0.0.1:18080/ui/
curl http://127.0.0.1:18080/api/v1/config
curl http://127.0.0.1:18080/api/v1/plugins
curl http://127.0.0.1:18080/api/v1/logs
```
