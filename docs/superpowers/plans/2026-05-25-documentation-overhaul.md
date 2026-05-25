# OpsAgent 文档全面升级 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a bilingual (zh/en) documentation system for OpsAgent, refreshing existing docs and adding missing ones (quickstart, config reference, API reference, architecture, contributing guide).

**Architecture:** Separate language directories (`docs/zh/` and `docs/en/`) with mirrored structure. Existing docs migrated via `git mv`. New docs written from source code analysis. Root `README.md` becomes a bilingual entry point.

**Tech Stack:** Markdown, git

---

## File Structure

```
opsagent/
├── README.md                    # Bilingual entry (links to zh/en)
├── README.zh.md                 # Chinese full README (refresh)
├── README.en.md                 # English full README (new)
├── CONTRIBUTING.md              # Contributing guide (new)
├── docs/
│   ├── README.md                # Bilingual doc index
│   ├── zh/                      # Chinese docs
│   │   ├── quickstart.md
│   │   ├── config-reference.md
│   │   ├── api-reference.md
│   │   ├── architecture.md
│   │   ├── platform-integration-guide.md  (moved)
│   │   ├── plugin-contract.md             (moved)
│   │   ├── sdk-development-guide.md       (moved)
│   │   ├── operations-guide.md            (moved)
│   │   ├── security-hardening.md          (moved)
│   │   ├── gateway-tunnel-guide.md        (moved+renamed)
│   │   └── changelog.md                   (moved)
│   └── en/                      # English docs (mirror)
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
```

---

### Task 1: Create directory structure and migrate existing docs

**Files:**
- Create: `docs/zh/`, `docs/en/`
- Move: `docs/*.md` → `docs/zh/*.md`

- [ ] **Step 1: Create directories**

```bash
mkdir -p docs/zh docs/en
```

- [ ] **Step 2: Move existing docs to zh/ using git mv**

```bash
git mv docs/platform-integration-guide.md docs/zh/
git mv docs/plugin-contract.md docs/zh/
git mv docs/sdk-development-guide.md docs/zh/
git mv docs/operations-guide.md docs/zh/
git mv docs/security-hardening.md docs/zh/
git mv docs/gateway-tunnel-platform-integration.md docs/zh/gateway-tunnel-guide.md
git mv docs/changelog.md docs/zh/
```

- [ ] **Step 3: Verify migration**

```bash
ls docs/zh/
# Expected: changelog.md  gateway-tunnel-guide.md  operations-guide.md
#           platform-integration-guide.md  plugin-contract.md
#           sdk-development-guide.md  security-hardening.md
```

- [ ] **Step 4: Commit**

```bash
git add docs/zh/ docs/en/
git commit -m "docs: create zh/en directories and migrate existing docs"
```

---

### Task 2: Write README.zh.md (Chinese full README)

**Files:**
- Create: `README.zh.md`
- Reference: `internal/config/config.go`, `internal/app/interfaces.go`

Based on current `README.md` with these additions:

- [ ] **Step 1: Create README.zh.md with updated capability table**

Add these rows to the capability table:

```
| 网关/跳板机 | TCP 隧道中继 + SSH 代理，支持 NAT 穿透的内部主机访问 |
| 系统健康检查 | 5 类 18 项检查：内核、文件系统、网络、服务、容器 |
```

- [ ] **Step 2: Update architecture diagram**

Add Gateway and Checker subsystems to the ASCII art:

```
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐  │
│  │  Gateway     │  │  Checker     │  │              │  │
│  │  (tunnel+    │  │  (kernel,fs, │  │              │  │
│  │   ssh proxy) │  │   net,svc,   │  │              │  │
│  │             │  │   container) │  │              │  │
│  └─────────────┘  └──────────────┘  └──────────────┘  │
```

- [ ] **Step 3: Update project structure**

Add to the tree:

```
│   ├── gateway/                # 跳板机网关（TCP 隧道 + SSH 代理）
│   │   ├── tunnel/             #   隧道池与中继
│   │   └── proxy/              #   SSH 代理客户端
│   ├── checker/                # 系统健康检查
│   │   ├── kernel/             #   内核检查（sysctl, version, module, boot_param）
│   │   ├── filesystem/         #   文件系统检查（权限, 挂载, 文件存在）
│   │   ├── network/            #   网络检查（端口, SSH 配置, iptables）
│   │   ├── service/            #   服务检查（systemd, PAM, cron, 用户）
│   │   └── container/          #   容器检查（Docker, containerd, cgroup）
```

- [ ] **Step 4: Add gateway and checker config examples**

```yaml
# 网关/跳板机
gateway:
  enabled: false
  listen_addr: ":18081"
  max_tunnels: 100
  tunnel_timeout_seconds: 30
  idle_timeout_seconds: 300
  hosts:
    - id: "internal-server-01"
      addr: "192.168.1.100:22"
      mode: "proxy"
      ssh:
        user: "admin"
        key_file: "/etc/opsagent/keys/id_rsa"
        port: 22

# 系统健康检查
checker:
  enabled: true
  max_concurrent: 5
  default_timeout_seconds: 30
  disabled_checkers: []
```

- [ ] **Step 5: Update security boundary section**

Add items 11-13:

```
11. Gateway PSK 认证 + SSH host key 验证
12. SSH 参数转义防止命令注入
13. crypto/rand 生成隧道 ID（不可预测）
```

- [ ] **Step 6: Update docs table to link to zh/ paths**

```markdown
| 文档 | 说明 |
|------|------|
| [快速入门教程](docs/zh/quickstart.md) | 从零开始的 step-by-step 教程 |
| [配置参考](docs/zh/config-reference.md) | 完整配置字段说明 |
| [API 参考](docs/zh/api-reference.md) | HTTP 端点完整参考 |
| [架构设计](docs/zh/architecture.md) | 系统架构与设计决策 |
| [平台集成指南](docs/zh/platform-integration-guide.md) | 平台侧 gRPC 服务实现 |
| [插件协议规范](docs/zh/plugin-contract.md) | UDS JSON-RPC 2.0 协议 |
| [SDK 开发指南](docs/zh/sdk-development-guide.md) | Go/Rust 插件 SDK |
| [运维部署指南](docs/zh/operations-guide.md) | 部署、监控、故障排查 |
| [安全加固手册](docs/zh/security-hardening.md) | 安全架构与加固措施 |
| [网关集成指南](docs/zh/gateway-tunnel-guide.md) | 跳板机/隧道平台集成 |
| [CHANGELOG](docs/zh/changelog.md) | 版本变更历史 |
```

- [ ] **Step 7: Commit**

```bash
git add README.zh.md
git commit -m "docs: add Chinese README with gateway/checker updates"
```

---

### Task 3: Write README.en.md (English full README)

**Files:**
- Create: `README.en.md`
- Reference: `README.zh.md`

- [ ] **Step 1: Translate README.zh.md to English**

Full translation of all sections. Maintain identical structure. Key translations:

| Chinese | English |
|---------|---------|
| 核心能力 | Core Capabilities |
| 架构 | Architecture |
| 内置插件 | Built-in Plugins |
| 快速启动 | Quick Start |
| 配置 | Configuration |
| API | API |
| Makefile 目标 | Makefile Targets |
| 打包与安装 | Packaging & Installation |
| 安全边界 | Security Boundaries |
| CI/CD | CI/CD |
| 平台集成 | Platform Integration |
| 项目结构 | Project Structure |
| 文档 | Documentation |

- [ ] **Step 2: Commit**

```bash
git add README.en.md
git commit -m "docs: add English README"
```

---

### Task 4: Write root README.md (bilingual entry)

**Files:**
- Overwrite: `README.md`

- [ ] **Step 1: Replace README.md with bilingual entry point**

```markdown
# OpsAgent

[![CI](https://github.com/cy77cc/opsagent/actions/workflows/ci.yml/badge.svg)](https://github.com/cy77cc/opsagent/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)

Host-side execution and metrics collection agent for the OpsPilot platform.

/ 主机侧执行与指标采集 Agent，面向 OpsPilot 控制面。

---

**[English](README.en.md)** | **[中文](README.zh.md)**

---

## Core Capabilities / 核心能力

| Capability | Description |
|-----------|-------------|
| Metrics Pipeline | 20 built-in plugins (10 Input + 3 Processor + 4 Aggregator + 3 Output) |
| Sandbox Execution | nsjail PID/NET/MNT namespace isolation + cgroup v2 |
| gRPC Streaming | Bidirectional streaming to platform (register, heartbeat, metrics, commands) |
| Plugin System | Rust UDS JSON-RPC runtime + custom plugin gateway |
| Gateway/Tunnel | TCP tunnel relay + SSH proxy for internal hosts |
| Health Checking | 5 categories, 18 checkers (kernel, filesystem, network, service, container) |
| Hot Reload | SIGHUP + gRPC config push with atomic rollback |
| Security | Command whitelist, seccomp, audit logging, mTLS, bearer auth |

## Quick Start / 快速启动

```bash
make tidy && make build
./bin/opsagent run --config ./configs/config.yaml
curl http://127.0.0.1:18080/healthz
```

## Documentation / 文档

| | English | 中文 |
|---|---------|------|
| Quick Start | [quickstart.md](docs/en/quickstart.md) | [quickstart.md](docs/zh/quickstart.md) |
| Config Reference | [config-reference.md](docs/en/config-reference.md) | [config-reference.md](docs/zh/config-reference.md) |
| API Reference | [api-reference.md](docs/en/api-reference.md) | [api-reference.md](docs/zh/api-reference.md) |
| Architecture | [architecture.md](docs/en/architecture.md) | [architecture.md](docs/zh/architecture.md) |
| Platform Integration | [guide](docs/en/platform-integration-guide.md) | [指南](docs/zh/platform-integration-guide.md) |
| Plugin Contract | [spec](docs/en/plugin-contract.md) | [规范](docs/zh/plugin-contract.md) |
| SDK Guide | [guide](docs/en/sdk-development-guide.md) | [指南](docs/zh/sdk-development-guide.md) |
| Operations | [guide](docs/en/operations-guide.md) | [指南](docs/zh/operations-guide.md) |
| Security | [hardening](docs/en/security-hardening.md) | [加固手册](docs/zh/security-hardening.md) |
| Gateway | [guide](docs/en/gateway-tunnel-guide.md) | [指南](docs/zh/gateway-tunnel-guide.md) |
| Changelog | [changelog.md](docs/en/changelog.md) | [changelog.md](docs/zh/changelog.md) |
| Contributing | [CONTRIBUTING.md](CONTRIBUTING.md) | [CONTRIBUTING.md](CONTRIBUTING.md) |

## License

[Apache License 2.0](LICENSE)
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: rewrite root README as bilingual entry point"
```

---

### Task 5: Write docs/zh/quickstart.md

**Files:**
- Create: `docs/zh/quickstart.md`
- Reference: `configs/config.yaml`, `internal/config/config.go`, `Makefile`

- [ ] **Step 1: Write quickstart tutorial**

Content structure:

```markdown
# 快速入门

## 前置条件

- Go 1.26+
- Linux（推荐 Ubuntu 22.04+）
- nsjail（可选，用于沙箱执行）
- cgroup v2（可选，用于资源限制）

## 获取源码

git clone https://github.com/cy77cc/opsagent.git
cd opsagent

## 编译

make tidy
make build
# 输出：bin/opsagent

## 配置

cp configs/config.yaml my-config.yaml
# 编辑 my-config.yaml，至少设置：
#   agent.id: "my-agent-001"
#   agent.name: "my-host"
#   grpc.server_addr: "your-platform:443"

## 运行

./bin/opsagent run --config my-config.yaml

## 验证

curl http://127.0.0.1:18080/healthz
curl http://127.0.0.1:18080/metrics

## 第一个指标采集

在 collector.inputs 中添加：
collector:
  inputs:
    - type: cpu
      config: { per_cpu: false }
    - type: memory
      config: {}

重启 agent 后查看：
curl http://127.0.0.1:18080/api/v1/metrics/latest

## 第一个沙箱执行

需要 nsjail + root 权限：
curl -X POST http://127.0.0.1:18080/api/v1/tasks \
  -H 'Content-Type: application/json' \
  -d '{"task_id":"t1","type":"sandbox_exec","payload":{"command":"echo","args":["hello"]}}'

## 连接平台

配置 gRPC 连接：
grpc:
  server_addr: "platform.example.com:443"
  enroll_token: "your-enroll-token"
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
```

- [ ] **Step 2: Commit**

```bash
git add docs/zh/quickstart.md
git commit -m "docs: add Chinese quickstart tutorial"
```

---

### Task 6: Write docs/zh/config-reference.md

**Files:**
- Create: `docs/zh/config-reference.md`
- Reference: `internal/config/config.go` (lines 11-191 for types, lines 194-249 for defaults)

- [ ] **Step 1: Write config reference document**

Extract all 13 config sections from `config.go`. For each field, document:
- Field name (YAML path)
- Type
- Default value (from `v.SetDefault()` calls)
- Required (from `Validate()`)
- Description

Sections to document:
1. `agent` — AgentConfig (id, name, interval_seconds, shutdown_timeout_seconds, audit_log)
2. `server` — ServerConfig (listen_addr)
3. `executor` — ExecutorConfig (timeout_seconds, allowed_commands, max_output_bytes)
4. `reporter` — ReporterConfig (mode, endpoint, timeout_seconds, retry_count, retry_interval_ms)
5. `auth` — AuthConfig (enabled, bearer_token)
6. `prometheus` — PrometheusConfig (enabled, path, protect_with_auth)
7. `grpc` — GRPCConfig (server_addr, enroll_token, mtls, heartbeat, reconnect backoffs, cache_persist_path)
8. `sandbox` — SandboxConfig (enabled, nsjail_path, base_workdir, timeouts, cgroup, audit, policy)
9. `plugin` — PluginConfig (enabled, runtime_path, socket_path, auto_start, timeouts, limits, sandbox_profile)
10. `plugin_gateway` — PluginGatewayConfig (enabled, plugins_dir, health_check, restarts, debounce)
11. `collector` — CollectorConfig (inputs[], processors[], aggregators[], outputs[])
12. `checker` — CheckerConfig (enabled, max_concurrent, default_timeout, disabled_checkers)
13. `gateway` — GatewayConfig (enabled, listen_addr, max_tunnels, timeouts, hosts[])

- [ ] **Step 2: Commit**

```bash
git add docs/zh/config-reference.md
git commit -m "docs: add Chinese config reference"
```

---

### Task 7: Write docs/zh/api-reference.md

**Files:**
- Create: `docs/zh/api-reference.md`
- Reference: `internal/server/handlers.go`, `internal/server/server.go`

- [ ] **Step 1: Write API reference document**

Document all 6 HTTP endpoints from `handlers.go`:

1. **GET /healthz** — Health check
   - Response: `{ success, data: { status, subsystems, version?, git_commit?, uptime_seconds? } }`
   - Status values: healthy, degraded, unhealthy
   - Subsystems: grpc, scheduler, plugin_runtime, gateway

2. **GET /readyz** — Readiness probe
   - Response 200: `{ success: true, data: { status: "ready" } }`
   - Response 503: `{ success: false, error: "collector not ready" }`

3. **POST /api/v1/exec** — Command execution
   - Request: `{ command, args, timeout_seconds }`
   - Response: `{ success, data: { stdout, stderr, exit_code, duration_ms } }`

4. **POST /api/v1/tasks** — Task execution (sandbox/plugin)
   - Request: `{ task_id, type, payload }`
   - Types: sandbox_exec, plugin_*
   - Payload varies by type

5. **GET /api/v1/metrics/latest** — Latest metrics snapshot
   - Response: `{ success, data: { metrics: [...] } }`

6. **GET /metrics** — Prometheus metrics (when enabled)

Include curl examples for each endpoint.

- [ ] **Step 2: Commit**

```bash
git add docs/zh/api-reference.md
git commit -m "docs: add Chinese API reference"
```

---

### Task 8: Write docs/zh/architecture.md

**Files:**
- Create: `docs/zh/architecture.md`
- Reference: `internal/app/interfaces.go`, `internal/app/agent.go`

- [ ] **Step 1: Write architecture document**

Content structure:

1. **整体架构** — ASCII diagram showing all subsystems and their connections
2. **子系统职责** — One section per subsystem:
   - Collector Pipeline: Input → Processor → Aggregator → Output
   - Sandbox Executor: nsjail + cgroup v2 + seccomp
   - gRPC Client: bidirectional streaming, reconnect, offline cache
   - Plugin Runtime: Rust UDS JSON-RPC
   - Plugin Gateway: plugin.yaml discovery, lifecycle, health checks
   - Gateway: TCP tunnel + SSH proxy
   - Checker: 5 categories, registry/executor pattern
   - HTTP Server: health, metrics, tasks, exec
   - Config Reloader: SIGHUP + gRPC, diff detection, atomic rollback
   - Audit Logger: JSON-lines, lumberjack rotation
3. **数据流** — Sequence diagrams for:
   - Metric collection flow: Input → Gather → Processor → Aggregator → Output → gRPC
   - Command execution flow: HTTP/gRPC → Dispatcher → Executor/Sandbox → Response
   - Plugin task flow: HTTP/gRPC → Dispatcher → PluginGateway → UDS RPC → Response
4. **设计决策** — Why nsjail, why gRPC bidirectional streaming, why Rust for plugin runtime

- [ ] **Step 2: Commit**

```bash
git add docs/zh/architecture.md
git commit -m "docs: add Chinese architecture document"
```

---

### Task 9: Write CONTRIBUTING.md

**Files:**
- Create: `CONTRIBUTING.md`
- Reference: `AGENTS.md`, `.github/workflows/ci.yml`, `Makefile`

- [ ] **Step 1: Write contributing guide**

Content structure:

```markdown
# Contributing to OpsAgent

## Development Environment

- Go 1.26+
- Rust (for plugin runtime)
- nsjail + cgroup v2 (for sandbox tests)
- golangci-lint

## Getting Started

1. Fork and clone
2. make tidy
3. make test-race

## Code Standards

- gofmt + goimports mandatory
- Functions < 50 lines
- Files < 800 lines
- Errors wrapped with context: fmt.Errorf("...: %w", err)
- Interfaces at usage site, not implementation site

## Testing

- 80% coverage minimum
- Table-driven tests
- Race detector required: make test-race
- Integration tests: make integration

## Commit Convention

<type>: <description>

Types: feat, fix, refactor, docs, test, chore, perf, ci

## Pull Request Process

1. Create feature branch from main
2. Write tests first (TDD encouraged)
3. Ensure make ci passes
4. Request review
5. Squash merge after approval

## Security

- No hardcoded secrets
- Command execution must go through whitelist
- Use context for timeouts
- Run gosec before submitting
```

- [ ] **Step 2: Commit**

```bash
git add CONTRIBUTING.md
git commit -m "docs: add contributing guide"
```

---

### Task 10: Write English docs (docs/en/)

**Files:**
- Create: `docs/en/*.md` (7 files mirroring docs/zh/)

- [ ] **Step 1: Translate existing zh/ docs to en/**

Translate these files from `docs/zh/` to `docs/en/`:
- `platform-integration-guide.md`
- `plugin-contract.md`
- `sdk-development-guide.md`
- `operations-guide.md`
- `security-hardening.md`
- `gateway-tunnel-guide.md`
- `changelog.md`

- [ ] **Step 2: Translate new zh/ docs to en/**

Translate these files from `docs/zh/` to `docs/en/`:
- `quickstart.md`
- `config-reference.md`
- `api-reference.md`
- `architecture.md`

- [ ] **Step 3: Commit**

```bash
git add docs/en/
git commit -m "docs: add English documentation set"
```

---

### Task 11: Update docs/README.md index

**Files:**
- Overwrite: `docs/README.md`

- [ ] **Step 1: Rewrite as bilingual index**

```markdown
# OpsAgent Documentation

**[English](#english)** | **[中文](#中文)**

---

## English

### User Documentation

| Document | Description |
|----------|-------------|
| [Quick Start](en/quickstart.md) | Step-by-step tutorial from zero |
| [Config Reference](en/config-reference.md) | Complete configuration field reference |
| [API Reference](en/api-reference.md) | HTTP endpoint reference |
| [Architecture](en/architecture.md) | System architecture and design decisions |
| [Platform Integration](en/platform-integration-guide.md) | gRPC service implementation guide |
| [Plugin Contract](en/plugin-contract.md) | UDS JSON-RPC 2.0 protocol spec |

### Developer Documentation

| Document | Description |
|----------|-------------|
| [SDK Guide](en/sdk-development-guide.md) | Go/Rust plugin SDK |
| [Contributing](../CONTRIBUTING.md) | Development setup and PR process |

### Operations Documentation

| Document | Description |
|----------|-------------|
| [Operations Guide](en/operations-guide.md) | Deployment, monitoring, troubleshooting |
| [Security Hardening](en/security-hardening.md) | Security architecture and hardening |
| [Gateway Guide](en/gateway-tunnel-guide.md) | Gateway/tunnel platform integration |

### Other

| Document | Description |
|----------|-------------|
| [Changelog](en/changelog.md) | Version history |
| [Archive](archive/) | Archived design specs and plans |

---

## 中文

### 用户文档

| 文档 | 说明 |
|------|------|
| [快速入门](zh/quickstart.md) | 从零开始的 step-by-step 教程 |
| [配置参考](zh/config-reference.md) | 完整配置字段说明 |
| [API 参考](zh/api-reference.md) | HTTP 端点完整参考 |
| [架构设计](zh/architecture.md) | 系统架构与设计决策 |
| [平台集成指南](zh/platform-integration-guide.md) | 平台侧 gRPC 服务实现 |
| [插件协议规范](zh/plugin-contract.md) | UDS JSON-RPC 2.0 协议 |

### 开发者文档

| 文档 | 说明 |
|------|------|
| [SDK 开发指南](zh/sdk-development-guide.md) | Go/Rust 插件 SDK |
| [贡献指南](../CONTRIBUTING.md) | 开发环境与 PR 流程 |

### 运维文档

| 文档 | 说明 |
|------|------|
| [运维部署指南](zh/operations-guide.md) | 部署、监控、故障排查 |
| [安全加固手册](zh/security-hardening.md) | 安全架构与加固措施 |
| [网关集成指南](zh/gateway-tunnel-guide.md) | 跳板机/隧道平台集成 |

### 其他

| 文档 | 说明 |
|------|------|
| [变更记录](zh/changelog.md) | 版本变更历史 |
| [归档文档](archive/) | 归档的设计规格与实现计划 |
```

- [ ] **Step 2: Commit**

```bash
git add docs/README.md
git commit -m "docs: rewrite docs index as bilingual hub"
```

---

### Task 12: Update existing zh/ docs with new features

**Files:**
- Modify: `docs/zh/operations-guide.md`
- Modify: `docs/zh/security-hardening.md`
- Modify: `docs/zh/changelog.md`

- [ ] **Step 1: Update operations-guide.md**

Add a Gateway deployment section covering:
- Gateway config (`gateway.enabled`, `listen_addr`, `hosts`)
- PSK authentication setup
- SSH key management for proxy mode
- Firewall rules (port 18081)

- [ ] **Step 2: Update security-hardening.md**

Add Gateway security section:
- PSK authentication for tunnel connections
- SSH host key verification (InsecureIgnoreHostKey disabled)
- crypto/rand tunnel ID generation
- SSH argument escaping to prevent injection

- [ ] **Step 3: Update changelog.md**

Add entries for:
- Gateway/Tunnel subsystem (2026-05-23)
- System health checker (2026-05-07)
- Security hardening sprint (2026-05-25)

- [ ] **Step 4: Commit**

```bash
git add docs/zh/operations-guide.md docs/zh/security-hardening.md docs/zh/changelog.md
git commit -m "docs: update existing docs with gateway/checker/security content"
```

---

### Task 13: Final verification

- [ ] **Step 1: Verify all links**

```bash
# Check that all referenced files exist
for f in docs/zh/*.md docs/en/*.md; do
  [ -f "$f" ] || echo "MISSING: $f"
done
```

- [ ] **Step 2: Verify zh/en parity**

```bash
# Both directories should have the same files
diff <(ls docs/zh/) <(ls docs/en/)
```

- [ ] **Step 3: Run full test suite to ensure no breakage**

```bash
make test-race
```

- [ ] **Step 4: Final commit if needed**

```bash
git add -A
git commit -m "docs: final documentation verification fixes"
```
