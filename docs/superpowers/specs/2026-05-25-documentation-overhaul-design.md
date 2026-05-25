# OpsAgent 文档全面升级设计规格

## Context

OpsAgent 已有较完善的中文文档体系（README 407行 + docs/ 下7个主要文档），但存在以下问题：

1. **内容滞后**：README 和现有文档未反映最近开发的 Gateway（跳板机/隧道）、Checker（5类健康检查）、安全加固等新功能
2. **缺失文档**：无 CONTRIBUTING.md、无独立 API 参考、无配置参考、无快速入门教程、无架构设计文档
3. **单语言**：仅有中文，缺乏英文版本
4. **结构扁平**：所有文档平铺在 docs/ 下，无语言分区

本次升级目标：刷新现有内容、补充缺失文档、创建双语文档体系。

## 目标文档结构

```
opsagent/
├── README.md                    # 双语入口（中/英切换链接）
├── README.zh.md                 # 中文版 README（刷新）
├── README.en.md                 # 英文版 README（新建）
├── CONTRIBUTING.md              # 贡献指南（新建）
├── docs/
│   ├── README.md                # 文档索引（双语链接）
│   ├── zh/                      # 中文文档集
│   │   ├── quickstart.md        # 快速入门教程
│   │   ├── config-reference.md  # 配置参考
│   │   ├── api-reference.md     # API 参考
│   │   ├── architecture.md      # 架构设计
│   │   ├── platform-integration-guide.md
│   │   ├── plugin-contract.md
│   │   ├── sdk-development-guide.md
│   │   ├── operations-guide.md
│   │   ├── security-hardening.md
│   │   ├── gateway-tunnel-guide.md
│   │   └── changelog.md
│   └── en/                      # 英文文档集（镜像结构）
│       ├── quickstart.md
│       ├── config-reference.md
│       ├── api-reference.md
│       ├── architecture.md
│       ├── platform-integration-guide.md
│       ├── plugin-contract.md
│       ├── sdk-development-guide.md
│       ├── operations-guide.md
│       ├── security-hardening.md
│       ├── gateway-tunnel-guide.md
│       └── changelog.md
├── archive/                     # 归档文档（保留不动）
└── superpowers/                 # 设计规格（保留不动）
```

## 各文档详细规格

### 1. README.md（双语入口）

**职责**：项目门面，提供中/英切换链接，简要项目概述

**内容**：
- 项目名称 + 一句话描述
- 徽章区（CI status、Go version、License）
- 中/英切换链接
- 核心能力概览表（精简版，5-8 行）
- 快速启动（3 步：build → configure → run）
- 文档索引链接

### 2. README.zh.md / README.en.md（完整 README）

**职责**：完整的项目文档入口

**内容刷新点**：
- 能力表：补充 Gateway（跳板机/隧道/SSH代理）、Checker（5类18项健康检查）
- 架构图：补充 Gateway 和 Checker 子系统
- 项目结构：补充 `internal/gateway/`、`internal/checker/` 目录
- 内置插件表：保持现有 20 个插件
- 配置示例：补充 `gateway` 和 `checker` 配置段
- API 示例：保持现有端点
- Makefile 目标：保持现有
- 打包安装：保持现有
- 安全边界：补充 Gateway 安全措施（PSK认证、SSH host key验证、参数转义）
- CI/CD：保持现有
- 文档索引：更新为新的双语文档结构

### 3. quickstart.md（快速入门教程）

**职责**：从零开始的 step-by-step 教程

**内容**：
1. 前置条件（Go 1.26+, nsjail, cgroup v2）
2. 获取源码
3. 编译（make build）
4. 配置（最小可用配置）
5. 运行（./bin/opsagent run）
6. 验证（curl healthz, metrics）
7. 第一个指标采集（配置 CPU + memory input）
8. 第一个沙箱执行（echo 命令）
9. 连接平台（gRPC 配置）

**数据来源**：
- `configs/config.yaml` 的默认值
- `internal/config/config.go` 的 Load() 函数
- `internal/server/handlers.go` 的端点定义

### 4. config-reference.md（配置参考）

**职责**：完整配置字段说明

**内容**：13 个配置段逐一说明

| 配置段 | 字段数 | 数据来源 |
|--------|--------|----------|
| agent | ~8 | `internal/config/config.go` AgentConfig |
| server | ~3 | ServerConfig |
| executor | ~4 | ExecutorConfig |
| reporter | ~6 | ReporterConfig |
| auth | ~2 | AuthConfig |
| prometheus | ~3 | PrometheusConfig |
| grpc | ~10 | GRPCConfig（含 mTLS 子结构） |
| sandbox | ~10 | SandboxConfig（含 policy 子结构） |
| plugin | ~9 | PluginConfig（含 sandbox_profile） |
| plugin_gateway | ~6 | PluginGatewayConfig |
| collector | ~4 | CollectorConfig（inputs/processors/aggregators/outputs） |
| checker | ~4 | CheckerConfig |
| gateway | ~6 | GatewayConfig（含 hosts 子结构） |

每个字段包含：名称、类型、默认值、是否必填、说明、示例

### 5. api-reference.md（API 参考）

**职责**：HTTP 端点完整参考

**内容**：

| 端点 | 方法 | 说明 | 数据来源 |
|------|------|------|----------|
| /healthz | GET | 健康检查 | `internal/server/handlers.go` |
| /readyz | GET | 就绪探针 | `internal/server/handlers.go` |
| /api/v1/exec | POST | 命令执行 | `internal/server/handlers.go` |
| /api/v1/tasks | POST | 任务执行（沙箱/插件） | `internal/server/handlers.go` |
| /api/v1/metrics/latest | GET | 最新指标快照 | `internal/server/handlers.go` |
| /metrics | GET | Prometheus 指标 | `internal/server/handlers.go` |

每个端点包含：路径、方法、请求头、请求体（JSON schema）、响应体（JSON 示例）、错误码、curl 示例

### 6. architecture.md（架构设计）

**职责**：系统架构文档

**内容**：
1. 整体架构图（ASCII art）
2. 子系统职责说明
   - Collector Pipeline（Input → Processor → Aggregator → Output）
   - Sandbox Executor（nsjail + cgroup v2）
   - gRPC Client（双向流、重连、缓存）
   - Plugin Runtime（Rust UDS JSON-RPC）
   - Plugin Gateway（plugin.yaml 发现、生命周期管理）
   - Gateway/Tunnel（跳板机、TCP隧道、SSH代理）
   - Checker（5类健康检查）
   - HTTP Server（API + Prometheus）
   - Config Reloader（热重载）
   - Audit Logger（审计日志）
3. 数据流：指标采集流、命令执行流、插件任务流
4. 关键设计决策：为什么用 nsjail、为什么用 gRPC 双向流、为什么用 Rust 做插件 runtime

### 7. CONTRIBUTING.md（贡献指南）

**职责**：开发者参与指南

**内容**：
1. 开发环境搭建（Go, Rust, nsjail, cgroup v2）
2. 代码规范（Go style, 错误处理, 测试要求）
3. 提交规范（conventional commits）
4. PR 流程（分支策略, review 要求, CI 检查）
5. 测试要求（80% 覆盖率, race detector）
6. 安全检查（gosec, 无硬编码密钥）
7. 文档更新要求

### 8. 现有文档迁移

**迁移规则**：
- `docs/*.md` → `docs/zh/*.md`（使用 `git mv` 保留历史）
- 同时创建 `docs/en/*.md`（英文翻译）
- `docs/README.md` 重写为双语索引页
- `docs/archive/` 和 `docs/superpowers/` 保持原位不动

**内容刷新**：
- `operations-guide.md`：补充 Gateway 部署说明
- `security-hardening.md`：补充 Gateway 安全措施
- `changelog.md`：补充 Gateway/Checker/Security 条目

## 实施顺序

1. **Phase 1**：创建目录结构 + README 入口文件
2. **Phase 2**：刷新 README.zh.md + 创建 README.en.md
3. **Phase 3**：新建 4 个文档（quickstart, config-reference, api-reference, architecture）
4. **Phase 4**：新建 CONTRIBUTING.md
5. **Phase 5**：迁移现有文档到 zh/ 目录
6. **Phase 6**：创建英文版文档（en/ 目录）
7. **Phase 7**：更新 docs/README.md 索引页

## 验证方式

- 所有文档中的链接可正常访问
- 中英文文档内容一致
- 配置参考与 `internal/config/config.go` 同步
- API 参考与 `internal/server/handlers.go` 同步
- 架构图反映最新子系统（含 Gateway、Checker）
- README 中的能力表、项目结构与代码一致
