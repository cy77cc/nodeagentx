# Gateway Tunnel 设计规格

## 概述

为 OpsAgent 新增 Gateway 模块，使 B 主机上的 Agent 充当跳板机，让平台能够管理 B 可达但 A 无法直连的内网主机。平台对内网主机的所有操作与直连主机完全一致，跳板逻辑对平台透明。

## 背景

场景：平台部署在 A 主机，B 主机与 A 网络互通，B 上有大量虚拟机（内网主机），A 无法直接访问这些虚拟机。需要通过 B 的 Agent 充当跳板，使平台能像管理直连主机一样管理这些内网主机。

约束：
- A 与 B 之间复用现有 gRPC 双向流通道进行隧道数据传输，A 侧不新增端口
- B 需新增一个监听端口供 C 的 Agent 连入（隧道模式）
- 仅需 TCP 流量转发
- 内网主机规模 100+
- 内网主机可能有 Agent，也可能没有

## 架构

```
┌──────────────────────────────────────────────────────────────────┐
│                        OpsPilot 平台 (A)                         │
│                                                                  │
│  主机列表：                                                       │
│  ├── B    (Agent，标记为 gateway 类型)                            │
│  ├── C1   (跳板=B，平台感知为普通主机)                             │
│  ├── C2   (跳板=B，平台感知为普通主机)                             │
│  └── Cn   (跳板=B，平台感知为普通主机)                             │
│                                                                  │
│  对 C1 的任何操作（指标/命令/脚本）和直连主机无差别                  │
└──────────────┬───────────────────────────────────────────────────┘
               │ gRPC 双向流
               │
┌──────────────┴───────────────────────────────────────────────────┐
│                     OpsAgent (B 主机)                            │
│                                                                  │
│  Gateway Module：                                                │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  连接检测 → C 上有没有 Agent？                               │  │
│  │                                                            │  │
│  │  有 Agent ──► 隧道模式：C 的 Agent 连 B，B 透明转发到 A     │  │
│  │  无 Agent ──► 代理模式：B 代为采集指标、执行命令、上报平台    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                  │
│  现有模块不受影响                                                 │
└──────────────┬───────────────────────────────────────────────────┘
               │ TCP / SSH
               ▼
┌──────────────────────────────────────────────────────────────────┐
│  C1 (有 Agent)  │  C2 (无 Agent)  │  C3 (有 Agent)  │  ...      │
│  Agent → B 网关  │  B 代为 SSH     │  Agent → B 网关  │           │
└──────────────────────────────────────────────────────────────────┘
```

两种模式对平台完全透明：

| | 隧道模式（C 有 Agent） | 代理模式（C 无 Agent） |
|---|---|---|
| C 在平台的表现 | 普通受管主机 | 普通受管主机 |
| 指标采集 | C 自己采集，经 B 隧道上报 | B 代为采集上报 |
| 命令执行 | 平台发给 C，经 B 隧道转发 | B 代为执行上报 |
| 连接方式 | C 的 Agent → B → A | B → C (SSH) |

## 平台侧变更

### 主机注册新增 jump_host 字段

```proto
message RegisterRequest {
  string agent_id = 1;
  string hostname = 2;
  string ip = 3;
  string jump_host = 4;  // 新增：跳板机 Agent ID，为空表示直连
}
```

平台 UI 添加主机时新增"跳板机"下拉选项，列出所有已注册的 gateway 类型 Agent。

### 连接路由逻辑

平台维护路由表：`host_id → { direct | via gateway_agent_id }`

连接时：
- 直连：现有逻辑不变
- 跳板：通过 B 的 gRPC 双向流建立隧道，后续所有消息经隧道收发

平台不区分隧道模式和代理模式，对平台来说 C 就是普通主机。

### 注册流程

**隧道模式**：
1. 用户在平台添加主机 C，设置跳板=B
2. C 上的 Agent 启动，配置 `gateway_addr` 指向 B
3. C 的 Agent → B → OpenTunnel → 平台，建立双向隧道
4. C 的 Agent 通过隧道向平台发送 Register 请求（复用现有注册流程）
5. 平台完成注册，C 正常上线

**代理模式**：
1. 用户在平台添加主机 C，设置跳板=B，选择代理模式
2. 平台通过 B 的 gRPC 流发送 AddProxyHost 指令，告知 B 需要代理的主机信息
3. B 向 C 发起 SSH 连接验证可达性
4. 验证通过后，B 以 C 的身份向平台发送 Register 请求
5. 平台完成注册，C 上线
6. 后续平台发给 C 的所有请求，B 代为处理并返回

## B 侧 Agent 变更

### 新增模块结构

```
internal/
  gateway/
    gateway.go            # Gateway 模块入口，生命周期管理
    detector.go           # 检测内网主机是否有 Agent
    tunnel/
      tunnel.go           # TCP-over-gRPC 隧道实现
      pool.go             # 连接池管理
    proxy/
      proxy.go            # 代理模式：代为采集/执行
      collector.go        # SSH 远程指标采集
      executor.go         # SSH 远程命令执行
    config.go             # 内网主机配置
```

### 隧道模式（C 有 Agent）

```
C 的 Agent                    B 的 Agent                   平台 A
     │                           │                           │
     │── gRPC Connect ──────────►│                           │
     │   (目标: 平台 A)           │                           │
     │                           │── OpenTunnel(C) ─────────►│
     │                           │   (复用 B 的 gRPC 流)      │
     │                           │                           │
     │◄── Register ──────────────│◄═══ 双向隧道 ═══════════►│
     │   (经 B 转发到 A)          │    (TCP 数据透传)          │
     │                           │                           │
     │◄═══════════════════════════════════════════════════►│
     │         所有 gRPC 消息经 B 透明转发                     │
```

- C 的 Agent 配置 `gateway_addr: B的地址:端口`（代替直接配置平台地址）
- B 收到 C 的连接后，向平台发起 OpenTunnel 请求
- 平台返回 tunnel stream，B 把 C 的 TCP 数据与 tunnel stream 双向桥接
- 之后 C 的 Agent 和平台之间的所有 gRPC 通信都经此隧道透传

### 代理模式（C 无 Agent）

```
平台 A                        B 的 Agent                    C (无 Agent)
  │                               │                            │
  │── CollectMetrics(C) ────────►│                            │
  │   (平台以为发给 C 的 Agent)    │── SSH: top/free/df ───────►│
  │                               │◄── 命令输出 ───────────────│
  │◄── MetricsResponse ──────────│                            │
  │   (格式与正常 Agent 一致)      │                            │
  │                               │                            │
  │── ExecCommand(C, "uptime") ─►│                            │
  │                               │── SSH: uptime ────────────►│
  │                               │◄── 输出 ──────────────────│
  │◄── ExecResponse ─────────────│                            │
```

- B 为每台无 Agent 的内网主机维护 SSH 连接（或按需连接）
- B 收到平台发给 C 的请求后，通过 SSH 代为执行，结果按标准格式返回
- 平台完全无感知，以为在和 C 的 Agent 通信

### C 的 Agent 配置变更

```yaml
grpc:
  gateway_addr: "192.168.1.10:18081"   # 指向 B，代替直接配置平台地址
  enroll_token: "your-token"
  heartbeat_interval_seconds: 15
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
```

当 Agent 检测到 `gateway_addr` 时，连接目标改为 B 而不是平台。

## gRPC Proto 扩展

### 新增 RPC 方法

```proto
service OpsService {
  // 现有方法不变
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc StreamMessages(stream AgentMessage) returns (stream PlatformMessage);

  // 新增：隧道
  rpc OpenTunnel(OpenTunnelRequest) returns (stream TunnelChunk);
  rpc TunnelUpload(stream TunnelChunk) returns (TunnelAck);
}
```

### 新增消息类型

```proto
message OpenTunnelRequest {
  string tunnel_id = 1;
  string target_host = 2;
  int32  target_port = 3;
  int32  timeout_seconds = 4;
}

message TunnelChunk {
  string tunnel_id = 1;
  bytes  data = 2;
}

message TunnelAck {
  string tunnel_id = 1;
  bool   success = 2;
  string error = 3;
}
```

### 交互流程

```
平台 A                              B 的 Agent                 C (内网)
  │                                     │                        │
  │  1. 发现 C 需要跳板 B                │                        │
  │                                     │                        │
  │  2. OpenTunnelRequest ─────────────►│                        │
  │     {tunnel_id, target: C:22}       │                        │
  │                                     │  3. TCP connect C:22   │
  │                                     │───────────────────────►│
  │                                     │◄── connected ─────────│
  │◄── TunnelChunk(连接成功) ──────────│                        │
  │                                     │                        │
  │  ════════ 双向数据透传 ════════      │                        │
  │                                     │                        │
  │── TunnelUpload(data) ─────────────►│── TCP send ───────────►│
  │                                     │◄── TCP recv ──────────│
  │◄── TunnelChunk(data) ──────────────│                        │
  │                                     │                        │
  │  4. gRPC over tunnel 透明通信        │                        │
  │◄══════════════════════════════════════════════════════════►│
```

## 配置

### B 的 Agent 配置

```yaml
gateway:
  enabled: true
  listen_addr: ":18081"          # C 的 Agent 连入端口（隧道模式）
  max_tunnels: 100               # 最大隧道数
  tunnel_timeout_seconds: 30     # 隧道建立超时
  idle_timeout_seconds: 300      # 空闲隧道回收

  hosts:
    - id: "vm-web-01"
      addr: "192.168.122.100"
      mode: "proxy"              # proxy | auto
      ssh:
        user: "root"
        key_file: "/etc/opsagent/keys/id_rsa"
        port: 22
    - id: "vm-db-01"
      addr: "192.168.122.101"
      mode: "auto"               # auto = 自动检测有无 Agent
      ssh:
        user: "root"
        key_file: "/etc/opsagent/keys/id_rsa"
```

### C 的 Agent 配置（隧道模式）

```yaml
grpc:
  gateway_addr: "192.168.1.10:18081"
  enroll_token: "your-token"
  heartbeat_interval_seconds: 15
  mtls: { ... }
```

## 安全设计

1. **隧道鉴权**：C 连 B 使用 mTLS + enroll_token；B 连 A 使用现有 mTLS
2. **隧道隔离**：每个 tunnel_id 绑定唯一 C 主机，B 不允许 C 访问其他 C 的隧道，平台验证 tunnel_id 与目标主机匹配
3. **代理模式安全**：SSH 密钥只存 B 本地，代理执行走现有白名单策略，操作审计日志记录来源
4. **隧道限流**：最大隧道数限制、单隧道带宽/连接数限制、空闲隧道自动回收

## 异常处理

| 场景 | 处理方式 |
|---|---|
| B 与 A 断连 | 隧道全断，C 的 Agent 自动重连 B，B 重连 A 后重建隧道 |
| B 与 C 断连 | 对应隧道关闭，平台感知为该主机离线（与直连主机断连表现一致） |
| 隧道建连超时 | 平台侧标记该主机连接失败，心跳超时后显示离线 |
| B 重启 | 隧道丢失，C 的 Agent 重连 B 触发隧道重建，代理模式配置持久化在 B 的配置文件中 |
| C 无 Agent 且 SSH 不通 | B 上报该主机不可达，平台显示离线 |

## 可观测性

### Prometheus 指标

```
opsagent_gateway_tunnels_active          # 当前活跃隧道数
opsagent_gateway_tunnel_bytes_total      # 隧道流量统计
opsagent_gateway_tunnel_errors_total     # 隧道错误计数
opsagent_gateway_proxy_requests_total    # 代理模式请求数
opsagent_gateway_proxy_latency_seconds   # 代理执行延迟
```

### 审计日志

```json
{"event": "gateway.tunnel.open", "tunnel_id": "xxx", "target": "192.168.122.100:22", "source": "vm-web-01"}
{"event": "gateway.tunnel.close", "tunnel_id": "xxx", "reason": "idle_timeout", "bytes": 12345}
{"event": "gateway.proxy.exec", "target": "vm-db-01", "command": "uptime", "success": true}
```

### 健康检查

B 的 `/healthz` 增加 gateway 子系统状态：

```json
{
  "status": "healthy",
  "subsystems": {
    "collector": "healthy",
    "grpc": "healthy",
    "gateway": "healthy",
    "gateway_tunnels": 15,
    "gateway_proxies": 8
  }
}
```
