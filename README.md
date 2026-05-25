# OpsAgent

[![CI](https://github.com/cy77cc/opsagent/actions/workflows/ci.yml/badge.svg)](https://github.com/cy77cc/opsagent/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)

Host-side execution and metrics collection agent for the OpsPilot platform.

主机侧执行与指标采集 Agent，面向 OpsPilot 控制面。

---

**[English](README.en.md)** | **[中文](README.zh.md)**

---

## Core Capabilities / 核心能力

| Capability / 能力 | Description / 说明 |
|---|---|
| Metrics Pipeline / 指标采集管线 | 20 built-in plugins (10 Input + 3 Processor + 4 Aggregator + 3 Output) |
| Sandbox Execution / 沙箱执行 | nsjail PID/NET/MNT namespace isolation + cgroup v2 resource limits |
| gRPC Streaming / gRPC 双向流 | Bidirectional streaming to platform (register, heartbeat, metrics, commands) |
| Plugin System / 插件系统 | Rust UDS JSON-RPC runtime + custom plugin gateway |
| Gateway / 网关 | TCP tunnel relay + SSH proxy for internal hosts behind NAT |
| Health Checking / 健康检查 | 5 categories, 18 checkers (kernel, filesystem, network, service, container) |
| Hot Reload / 配置热重载 | SIGHUP + gRPC config push with atomic rollback |
| Security / 安全 | Command whitelist, seccomp, audit logging, mTLS, bearer auth |

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
