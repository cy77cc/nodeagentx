# Spec: 插件生态扩展

> 日期: 2026-05-27
> 状态: Approved
> 作者: AI Assistant + User

## Context

OpsAgent 已实现 Telegraf 风格的四阶段管线（10 Input + 3 Processor + 4 Aggregator + 3 Output）和自定义插件网关（UDS JSON-RPC）。本 spec 定义插件生态扩展：新插件、WASM 运行时、插件市场、开发体验优化。

## 目标

1. 新增高价值 Input/Output 插件（journald、syslog、http、snmp、cloud_metadata、otlp）
2. WASM 插件运行时（基于 Wazero，纯 Go）
3. 最小可行插件市场（Git 注册表 + CLI 安装）
4. 插件开发体验优化（代码生成、测试工具、文档生成）

## 依赖

- 现有 Collector Pipeline（`internal/collector/`）
- 现有 Plugin Gateway（`internal/pluginruntime/gateway.go`）
- 现有 SDK（`sdk/plugin/`、`sdk/opsagent-plugin/`）

## 设计

### 1. 新增 Input/Output 插件

#### 1.1 P0 插件（高优先级）

| 插件 | 包路径 | 依赖 |
|------|--------|------|
| `inputs/journald` | `internal/collector/inputs/journald/` | `github.com/coreos/go-systemd/v22/sdjournal` |
| `inputs/syslog` | `internal/collector/inputs/syslog/` | 标准库 `net` |
| `inputs/http` | `internal/collector/inputs/http/` | 标准库 `net/http` |
| `inputs/snmp` | `internal/collector/inputs/snmp/` | `github.com/gosnmp/gosnmp` |
| `inputs/cloud_metadata` | `internal/collector/inputs/cloudmetadata/` | 标准库 `net/http` |
| `outputs/otlp` | `internal/collector/outputs/otlp/` | `go.opentelemetry.io/otel/exporters/otlp` |

#### 1.2 P1 插件（中优先级）

| 插件 | 包路径 | 依赖 |
|------|--------|------|
| `inputs/kubernetes` | `internal/collector/inputs/kubernetes/` | `k8s.io/client-go` |
| `inputs/jmx` | `internal/collector/inputs/jmx/` | HTTP bridge 协议 |
| `inputs/hwmon` | `internal/collector/inputs/hwmon/` | 解析 `lm-sensors` 输出 |
| `inputs/cron` | `internal/collector/inputs/cron/` | 解析 crontab + systemd timer |
| `inputs/ntp` | `internal/collector/inputs/ntp/` | 解析 `chronyc` 输出 |
| `inputs/prometheus` | `internal/collector/inputs/prometheus/` | HTTP + protobuf 解析 |
| `outputs/loki` | `internal/collector/outputs/loki/` | HTTP push API |

#### 1.3 P2 插件（探索性）

| 插件 | 包路径 | 依赖 |
|------|--------|------|
| `inputs/ebpf` | `internal/collector/inputs/ebpf/` | `github.com/cilium/ebpf` |
| `inputs/prometheus_remote_write` | `internal/collector/inputs/promrw_receiver/` | HTTP + protobuf |

#### 1.4 插件实现模式

所有新插件遵循现有模式：

```go
package httpinput

import (
    "context"
    "github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## HTTP Input 插件
## 示例配置：
# urls = ["http://localhost:8080/metrics"]
# method = "GET"
# timeout = 5
`

func init() {
    collector.RegisterInput("http", func() collector.Input {
        return &HTTPInput{}
    })
}

type HTTPInput struct {
    URLs    []string `toml:"urls"`
    Method  string   `toml:"method"`
    Timeout int      `toml:"timeout_seconds"`
    client  *http.Client
}

func (h *HTTPInput) Init(cfg map[string]interface{}) error {
    // 解析配置
    return nil
}

func (h *HTTPInput) Gather(ctx context.Context, acc collector.Accumulator) error {
    // 对每个 URL 发起请求，解析响应，写入 Accumulator
    return nil
}

func (h *HTTPInput) SampleConfig() string {
    return sampleConfig
}
```

### 2. WASM 插件运行时

#### 2.1 技术选型

**Wazero**（纯 Go，零 CGo）

| 因素 | Wazero | Wasmtime |
|------|--------|----------|
| 纯 Go | 是 | 否（Rust + CGo） |
| WASI Preview 1 | 完整支持 | 完整支持 |
| 构建复杂度 | 低 | 高 |
| 性能 | 良好 | 更好 |
| 与 OpsAgent 兼容 | 完美 | 需要 CGo 交叉编译 |

选择 Wazero 的理由：与 OpsAgent 全 Go 技术栈一致，零 CGo 依赖，构建和部署简单。

#### 2.2 架构

WASM 作为 Plugin Gateway 的新 runtime 类型：

```
Task Dispatcher
    ├── plugin_runtime (Rust UDS)      # 现有：内置 handler
    ├── plugin_gateway (Go/Rust 进程)  # 现有：自定义插件
    └── wasm_runtime (Wazero)          # 新增：WASM 插件
```

#### 2.3 核心组件

```go
// internal/wasm/runtime.go
type WASMRuntime struct {
    modules map[string]*WASMModule
    store   wazero.Runtime
    cache   wazero.CompilationCache
    logger  zerolog.Logger
}

type WASMModule struct {
    Name      string
    Path      string
    TaskTypes []string
    Config    map[string]interface{}
    instance  api.Module
}

// 通信方式：WASI stdin/stdout JSON-RPC
// 输入：stdin（JSON-RPC 请求）
// 输出：stdout（JSON-RPC 响应）
// 文件系统：受限挂载（/tmp/opsagent/wasm/{name}/）
```

#### 2.4 WASM 插件清单

```yaml
name: "my-wasm-plugin"
version: "1.0.0"
runtime: wasm                    # 现有：process，新增：wasm
binary_path: "./plugin.wasm"
task_types:
  - "transform"
  - "enrich"
limits:
  max_memory_pages: 256          # 16MB (256 * 64KB)
  max_execution_ms: 5000
sandbox:
  enabled: true                  # WASM 天然沙箱
  allowed_paths:
    - "/tmp/opsagent/wasm/my-wasm-plugin/"
```

#### 2.5 WASM SDK

新增 Rust crate `sdk/opsagent-wasm/`，基于现有 `sdk/opsagent-plugin/` 扩展：

```
sdk/opsagent-wasm/
├── Cargo.toml              # crate 配置，依赖 opsagent-plugin
├── src/
│   ├── lib.rs              # 公开 API：execute 入口点、宏
│   ├── host.rs             # Host 调用桥接（stdin/stdout JSON-RPC）
│   └── macros.rs           # #[opsagent_wasm_plugin] 宏
└── README.md
```

核心实现：

```rust
// sdk/opsagent-wasm/src/lib.rs
use opsagent_plugin::{TaskRequest, TaskResponse, Result};

/// WASM 插件入口点
/// 运行时通过 stdin 传入 JSON-RPC 请求，插件处理后通过 stdout 返回响应
#[no_mangle]
pub extern "C" fn execute(input_ptr: *const u8, input_len: usize) -> *mut u8 {
    // 1. 从 stdin 读取 JSON-RPC 请求
    // 2. 反序列化为 TaskRequest
    // 3. 调用用户实现的 handler
    // 4. 序列化 TaskResponse 到 stdout
}
```

**密钥管理**：WASM 插件签名使用与进程插件相同的 Ed25519 密钥对，公钥由平台维护并通过 registry/index.json 分发。

#### 2.6 配置

```yaml
wasm:
  enabled: false
  plugins_dir: "/etc/opsagent/wasm-plugins"
  max_modules: 10
  cache_dir: "/var/lib/opsagent/wasm-cache"
```

### 3. 插件市场（最小可行版）

#### 3.1 架构

Git 仓库 + JSON 索引 + CLI 安装工具。

#### 3.2 注册表格式

```json
// registry/index.json
{
  "version": "1.0.0",
  "plugins": [
    {
      "name": "nginx-monitor",
      "version": "1.2.0",
      "description": "Nginx metrics via stub_status and access log",
      "author": "opsagent-team",
      "type": "input",
      "runtime": "process",
      "platforms": ["linux/amd64", "linux/arm64"],
      "sha256": "abc123...",
      "signature": "MEUCIQD...",
      "download_url": "https://releases.example.com/plugins/nginx-monitor-1.2.0-linux-amd64",
      "min_agent_version": "1.0.0",
      "task_types": [],
      "config_schema": {}
    }
  ]
}
```

#### 3.3 CLI 命令

```bash
opsagent plugin search nginx            # 搜索插件
opsagent plugin info nginx-monitor      # 查看详情
opsagent plugin install nginx-monitor   # 安装插件
opsagent plugin update nginx-monitor    # 更新插件
opsagent plugin remove nginx-monitor    # 卸载插件
opsagent plugin list                    # 列出已安装
opsagent plugin verify nginx-monitor    # 验证签名
```

#### 3.4 安装流程

1. 从 `registry/index.json` 查找插件
2. 下载二进制到 `/etc/opsagent/plugins/{name}/`
3. 验证 SHA-256 校验和
4. 验证 Ed25519 签名
5. 解压并设置权限 (0755)
6. 写入 `plugin.yaml` 清单
7. Plugin Gateway 自动发现并加载

#### 3.5 安全要求

- 所有插件必须签名（Ed25519）
- SHA-256 校验和验证
- 插件沙箱可选启用（WASM 强制沙箱）
- 运行时资源限制（`limits` 字段）
- 审计日志记录（已有的 audit logger）

### 4. 插件开发体验

#### 4.1 代码生成 CLI

```bash
# 创建新 Input 插件骨架
opsagent plugin new --type input --name my-input
# 生成：
#   internal/collector/inputs/myinput/myinput.go
#   internal/collector/inputs/myinput/myinput_test.go

# 创建新自定义插件（Go）
opsagent plugin new --type custom --name my-plugin --lang go
# 生成：
#   sdk/examples/my-plugin/plugin.yaml
#   sdk/examples/my-plugin/main.go
#   sdk/examples/my-plugin/main_test.go

# 创建新 WASM 插件（Rust）
opsagent plugin new --type wasm --name my-wasm --lang rust
# 生成：
#   sdk/examples/my-wasm/plugin.yaml
#   sdk/examples/my-wasm/Cargo.toml
#   sdk/examples/my-wasm/src/lib.rs
```

#### 4.2 生成的插件骨架

```go
// internal/collector/inputs/myinput/myinput.go
package myinput

import (
    "context"
    "github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## My Input 插件
# option1 = "default_value"
# option2 = 42
`

func init() {
    collector.RegisterInput("myinput", func() collector.Input {
        return &MyInput{}
    })
}

type MyInput struct {
    Option1 string `toml:"option1"`
    Option2 int    `toml:"option2"`
}

func (m *MyInput) Init(cfg map[string]interface{}) error {
    return nil
}

func (m *MyInput) Gather(ctx context.Context, acc collector.Accumulator) error {
    return nil
}

func (m *MyInput) SampleConfig() string {
    return sampleConfig
}
```

#### 4.3 测试工具

```bash
# 测试单个插件
opsagent test --input myinput --config configs/test.yaml

# 验证插件接口实现
opsagent plugin validate myinput

# 生成插件文档
opsagent plugin docs myinput --output docs/plugins/myinput.md
```

#### 4.4 文档

- `docs/en/plugin-development-guide.md` — 完整开发指南
- `docs/en/plugin-testing-guide.md` — 测试最佳实践
- `docs/en/plugin-contract.md` — 接口契约（已有）

## 关键文件

| 文件 | 操作 |
|------|------|
| `internal/collector/inputs/journald/` | **新建** |
| `internal/collector/inputs/syslog/` | **新建** |
| `internal/collector/inputs/http/` | **新建** |
| `internal/collector/inputs/snmp/` | **新建** |
| `internal/collector/inputs/cloudmetadata/` | **新建** |
| `internal/collector/inputs/kubernetes/` | **新建** |
| `internal/collector/inputs/jmx/` | **新建** |
| `internal/collector/inputs/hwmon/` | **新建** |
| `internal/collector/inputs/cron/` | **新建** |
| `internal/collector/inputs/ntp/` | **新建** |
| `internal/collector/inputs/prometheus/` | **新建** |
| `internal/collector/inputs/ebpf/` | **新建** |
| `internal/collector/outputs/otlp/` | **新建** |
| `internal/collector/outputs/loki/` | **新建** |
| `internal/wasm/` | **新建** — WASM 运行时 |
| `internal/wasm/runtime.go` | **新建** |
| `internal/wasm/registry.go` | **新建** |
| `internal/pluginruntime/gateway.go` | **修改** — 集成 WASM runtime |
| `sdk/opsagent-wasm/` | **新建** — WASM SDK crate |
| `internal/app/commands.go` | **修改** — 新增 plugin 子命令 |
| `internal/app/commands.go` | **修改** — 新增 templates 子命令 |

## 测试要求

- 每个新 Input 插件：单元测试 + 集成测试
- WASM Runtime：模块加载、执行、超时、内存限制
- 插件市场：下载、签名验证、安装、卸载
- 代码生成：生成的骨架可编译通过

## 验证方式

```bash
# 新插件单元测试
go test -race ./internal/collector/inputs/http/...
go test -race ./internal/collector/inputs/snmp/...
go test -race ./internal/collector/outputs/otlp/...

# WASM 测试
go test -race ./internal/wasm/...

# CLI 验证
opsagent plugin new --type input --name test-input
opsagent plugin list

# 集成测试
go test -race -tags=integration ./internal/integration/ -run TestWASM
go test -race -tags=integration ./internal/integration/ -run TestPluginMarketplace
```
