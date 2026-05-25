# OpsAgent 快速入门

本指南帮助你在本地环境中编译、配置并运行 OpsAgent，完成第一次指标采集和沙箱执行。

## 1. 前置条件

| 依赖 | 版本要求 | 用途 |
|------|---------|------|
| Go | 1.26+ | 编译 |
| Git | 任意 | 获取源码 |
| Linux (Ubuntu 22.04+) | - | 运行环境 |
| nsjail | 3.0+（可选） | 沙箱执行 |
| cgroup v2 | 内核 5.2+（可选） | 沙箱资源限制 |

检查 Go 版本：

```bash
go version
# go version go1.26.1 linux/amd64
```

检查沙箱前置条件（可选）：

```bash
# 检查 nsjail 是否安装
which nsjail

# 检查 cgroup v2
test -f /sys/fs/cgroup/cgroup.controllers && echo "cgroup v2: OK" || echo "cgroup v2: 不可用"
```

## 2. 获取源码

```bash
git clone https://github.com/cy77cc/opsagent.git
cd opsagent
```

## 3. 编译

```bash
# 同步依赖
make tidy

# 编译
make build
```

编译产物位于 `bin/opsagent`，验证编译结果：

```bash
./bin/opsagent --help
```

> 如果需要交叉编译 amd64 和 arm64 两个架构的二进制，使用 `make build-all`。

## 4. 配置

复制默认配置文件作为起点：

```bash
cp configs/config.yaml my-config.yaml
```

**最低必填字段**（启动时必须提供，否则会报错退出）：

```yaml
agent:
  id: "agent-local-001"        # 唯一标识符
  name: "local-dev-agent"      # 可读名称

server:
  listen_addr: "127.0.0.1:18080"  # 本地 API 监听地址

executor:
  timeout_seconds: 10
  allowed_commands:
    - uptime
    - df
    - free

grpc:
  server_addr: "platform.example.com:443"  # 平台 gRPC 地址
```

> `agent.id`、`agent.name`、`server.listen_addr`、`executor.allowed_commands`（非空）和 `grpc.server_addr` 均为必填项。其余字段有合理默认值，无需立即设置。

## 5. 运行

```bash
./bin/opsagent run --config my-config.yaml
```

启动成功后，日志中会显示类似输出：

```
level=INFO msg="agent started" agent_id=agent-local-001 listen=127.0.0.1:18080
```

也可以直接通过 Makefile 运行（使用默认配置）：

```bash
make run
```

## 6. 验证服务

在另一个终端中执行：

```bash
# 健康检查
curl -s http://127.0.0.1:18080/healthz

# 查看 Prometheus 指标
curl -s http://127.0.0.1:18080/metrics
```

如果 `healthz` 返回 200 且 `metrics` 有 Prometheus 格式输出，说明服务已正常运行。

## 7. 第一次指标采集

编辑 `my-config.yaml`，配置采集器（`collector`）的 `inputs` 部分：

```yaml
collector:
  inputs:
    - type: cpu
      config:
        per_cpu: false
    - type: memory
      config: {}
  processors: []
  aggregators: []
  outputs: []
```

重启 OpsAgent：

```bash
# 停止当前进程 (Ctrl+C)，然后重新启动
./bin/opsagent run --config my-config.yaml
```

等待至少一个采集周期（默认 10 秒），然后查询最新采集的指标：

```bash
curl -s http://127.0.0.1:18080/api/v1/metrics/latest | python3 -m json.tool
```

你应当看到类似以下的 JSON 输出，包含 CPU 使用率和内存使用情况：

```json
{
  "metrics": [
    {
      "name": "cpu",
      "fields": {
        "usage_percent": 12.5
      },
      "tags": {},
      "timestamp": "2026-05-25T10:30:00Z"
    },
    {
      "name": "memory",
      "fields": {
        "total_bytes": 8589934592,
        "used_bytes": 4294967296,
        "usage_percent": 50.0
      },
      "tags": {},
      "timestamp": "2026-05-25T10:30:00Z"
    }
  ]
}
```

所有可用的 input 类型：

| 类型 | 说明 |
|------|------|
| `cpu` | CPU 使用率 |
| `memory` | 内存使用量 |
| `disk` | 磁盘使用量 |
| `net` | 网络流量 |
| `load` | 系统负载 |
| `diskio` | 磁盘 I/O |
| `temp` | 温度传感器 |
| `gpu` | GPU 指标（需要 nvidia-smi） |
| `connections` | 网络连接状态 |

## 8. 第一个沙箱执行

> **注意**：沙箱执行需要 nsjail 已安装，且 OpsAgent 以 root 权限运行。

编辑 `my-config.yaml`，启用沙箱：

```yaml
sandbox:
  enabled: true
  nsjail_path: "/usr/bin/nsjail"
  base_workdir: "/tmp/opsagent/sandbox"
  default_timeout_seconds: 30
  max_concurrent_tasks: 4
  cgroup_base_path: "/sys/fs/cgroup/opsagent"
  audit_log_path: "/var/log/opsagent/audit.log"
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
```

以 root 身份重启：

```bash
sudo ./bin/opsagent run --config my-config.yaml
```

提交一个沙箱任务：

```bash
curl -s -X POST http://127.0.0.1:18080/api/v1/tasks \
  -H "Content-Type: application/json" \
  -d '{
    "command": "echo",
    "args": ["hello from sandbox"],
    "timeout_seconds": 10
  }' | python3 -m json.tool
```

预期返回：

```json
{
  "task_id": "task-xxxx",
  "status": "completed",
  "stdout": "hello from sandbox\n",
  "stderr": "",
  "exit_code": 0
}
```

## 9. 连接平台（gRPC）

将 OpsAgent 注册到 OpsPilot 平台需要配置 gRPC 连接。生产环境建议启用 mTLS。

```yaml
grpc:
  server_addr: "platform.example.com:443"
  enroll_token: "your-enrollment-token-from-platform"
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
  heartbeat_interval_seconds: 15
  reconnect_initial_backoff_ms: 1000
  reconnect_max_backoff_ms: 30000
```

各字段说明：

| 字段 | 说明 |
|------|------|
| `server_addr` | 平台 gRPC 服务地址（必填） |
| `enroll_token` | 注册令牌，在平台创建 Agent 时获取 |
| `mtls.*` | 双向 TLS 证书路径（生产环境必填） |
| `heartbeat_interval_seconds` | 心跳间隔，默认 15 秒 |
| `reconnect_*` | 断线重连的退避策略 |

启用 mTLS 前，确保三个证书文件均已部署到目标路径。连接建立后，Agent 会自动通过心跳保持在线状态。

## 下一步

- [安全加固指南](security-hardening.md) -- 认证、鉴权与网络配置
- [运维指南](operations-guide.md) -- 日志、监控与故障排查
- [网关隧道指南](gateway-tunnel-guide.md) -- 通过跳板机访问内网主机
- [平台接入指南](platform-integration-guide.md) -- 完整的平台对接流程
