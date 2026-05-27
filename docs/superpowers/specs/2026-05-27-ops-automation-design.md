# Spec: 运维自动化

> 日期: 2026-05-27
> 状态: Approved
> 作者: AI Assistant + User

## Context

OpsAgent 已完整实现所有核心功能。本 spec 定义四个运维自动化功能：服务自发现、配置模板库、批量管理标签、自动更新。这些功能提升 Agent 的自治能力和大规模运维效率。

## 目标

1. 服务自发现：自动发现主机上的服务（systemd、端口、容器），上报平台
2. 配置模板库：内置常见服务的采集模板，CLI 和 gRPC 推送应用
3. 批量管理标签：Agent 注册时携带标签和分组，支持平台侧筛选和批量操作
4. 自动更新：平台通过 gRPC 推送更新指令，Agent 执行 A/B 二进制替换

## 依赖

- 现有 Collector Pipeline（`internal/collector/`）
- 现有 gRPC Client（`internal/grpcclient/`）
- 现有配置热重载（`internal/config/reload.go`）
- `gopsutil` 库（已有依赖，用于 /proc 检查）
- `fsnotify` 库（已有依赖，用于文件监听）

## 设计

### 1. 服务自发现

#### 1.1 架构

新增 `internal/discovery/` 包，多层发现策略，结果通过 gRPC 上报平台。

#### 1.2 发现层

| 层级 | 方法 | 发现内容 | 实现 |
|------|------|----------|------|
| Layer 1 | systemd 单元枚举 | 运行中的 service 及 PID/端口 | `systemctl list-units` + D-Bus |
| Layer 2 | /proc 端口扫描 | 监听端口 → 进程映射 | `gopsutil` `net.Connections()` |
| Layer 3 | 容器运行时 API | Docker/containerd 容器及端口 | 查询 Docker socket |
| Layer 4 | 云元数据 | 实例信息、区域、标签 | HTTP 请求 `169.254.169.254` |

#### 1.3 核心类型

```go
// internal/discovery/discovery.go
type DiscoveryService struct {
    interval   time.Duration
    layers     []DiscoveryLayer
    discovered []Service
    mu         sync.RWMutex
    logger     zerolog.Logger
}

type DiscoveryLayer interface {
    Name() string
    Discover(ctx context.Context) ([]Service, error)
}

type Service struct {
    Name         string            `json:"name"`          // nginx, postgres, redis
    Type         string            `json:"type"`          // systemd, container, process
    PID          int               `json:"pid"`
    Ports        []int             `json:"ports"`
    Labels       map[string]string `json:"labels"`        // Docker labels, K8s annotations
    Metadata     map[string]string `json:"metadata"`      // 版本、镜像等
    DiscoveredAt time.Time         `json:"discovered_at"`
}
```

#### 1.4 systemd 发现层

```go
// internal/discovery/systemd.go
type SystemdLayer struct{}

func (s *SystemdLayer) Discover(ctx context.Context) ([]Service, error) {
    // 1. systemctl list-units --type=service --state=running --no-legend
    // 2. 解析输出获取 unit 名称和 PID
    // 3. systemctl show <unit> -p MainPID 获取 PID
    // 4. gopsutil net.ConnectionsPid() 获取端口
    // 5. 从 unit 名推断服务类型（nginx → nginx, postgresql → postgres）
}
```

#### 1.5 /proc 发现层

```go
// internal/discovery/proc.go
type ProcLayer struct{}

func (p *ProcLayer) Discover(ctx context.Context) ([]Service, error) {
    // 1. gopsutil net.Connections("inet") 获取所有监听端口
    // 2. 按 PID 分组
    // 3. 从 /proc/<pid>/cmdline 获取进程名
    // 4. 从 /proc/<pid>/exe 符号链接获取二进制路径
    // 5. 已知进程名映射到服务类型
}
```

#### 1.6 容器发现层

```go
// internal/discovery/container.go
type ContainerLayer struct {
    dockerSocket string
}

func (c *ContainerLayer) Discover(ctx context.Context) ([]Service, error) {
    // 1. 检查 /var/run/docker.sock 是否存在
    // 2. GET /containers/json API 列出运行中容器
    // 3. 提取容器名、镜像、端口映射、labels
    // 4. 类似检查 containerd CRI socket
}
```

#### 1.7 发现映射表

内置映射（可扩展）：

| 发现的服务 | 自动建议的 Input |
|-----------|-----------------|
| nginx | `inputs.tail` (access.log) + `inputs.http` (stub_status) |
| postgres | `inputs.process` (连接数) |
| redis | `inputs.process` (内存/连接) |
| docker | `inputs.connections` (容器网络) |
| mysql | `inputs.process` (连接数) |
| sshd | `inputs.tail` (auth.log) |

#### 1.8 gRPC 消息

```protobuf
message ServiceDiscoveryReport {
    string agent_id = 1;
    repeated ServiceInfo services = 2;
    int64 timestamp = 3;
}

message ServiceInfo {
    string name = 1;
    string type = 2;
    int32 pid = 3;
    repeated int32 ports = 4;
    map<string, string> labels = 5;
    map<string, string> metadata = 6;
}
```

#### 1.9 配置

```yaml
discovery:
  enabled: true
  interval_seconds: 300
  layers:
    - type: "systemd"
      enabled: true
    - type: "proc"
      enabled: true
    - type: "container"
      enabled: true
      docker_socket: "/var/run/docker.sock"
    - type: "metadata"
      enabled: false
  auto_configure:
    enabled: false
    suggestion_only: true
```

### 2. 配置模板库

#### 2.1 架构

Go `embed.FS` 嵌入模板文件，CLI 命令 + 平台 gRPC 推送。

#### 2.2 模板目录

```
internal/templates/
├── embed.go
├── templates/
│   ├── nginx.yaml
│   ├── postgres.yaml
│   ├── redis.yaml
│   ├── docker.yaml
│   ├── mysql.yaml
│   ├── mongodb.yaml
│   ├── rabbitmq.yaml
│   ├── elasticsearch.yaml
│   ├── system.yaml
│   └── kubernetes.yaml
├── loader.go
└── loader_test.go
```

#### 2.3 模板格式

```yaml
# templates/nginx.yaml
name: "nginx"
description: "Nginx web server monitoring"
version: "1.0.0"
requires:
  - service: "nginx"
  - port: 80

variables:
  stub_status_url:
    description: "Nginx stub_status endpoint URL"
    default: "http://127.0.0.1:80/nginx_status"
    type: "string"
  log_path:
    description: "Nginx access log path"
    default: "/var/log/nginx/access.log"
    type: "string"

collector:
  inputs:
    - type: http
      config:
        urls: ["{{.stub_status_url}}"]
        method: "GET"
        name_override: "nginx_status"
    - type: tail
      config:
        files: ["{{.log_path}}"]
        from_beginning: false
  processors:
    - type: logparse
      config:
        rules:
          - field: "message"
            parser: "grok"
            grok_pattern: '%{IPORHOST:client_ip} ...'
  outputs: []
```

#### 2.4 CLI 命令

```bash
opsagent templates list                    # 列出可用模板
opsagent templates show nginx              # 查看模板详情
opsagent templates apply nginx --var ...   # 应用模板
opsagent templates suggest                 # 根据发现结果推荐
```

#### 2.5 gRPC 推送

```protobuf
message ApplyTemplate {
    string template_name = 1;
    map<string, string> variables = 2;
    bool dry_run = 3;
}
```

### 3. 批量管理标签

#### 3.1 Proto 扩展

```protobuf
message AgentRegistration {
    string agent_id = 1;
    string hostname = 2;
    string ip = 3;
    string version = 4;
    int64 startup_time = 5;
    map<string, string> labels = 6;
    repeated string groups = 7;
    map<string, string> capabilities = 8;
}

message AgentHeartbeat {
    string agent_id = 1;
    int64 timestamp = 2;
    string status = 3;
    map<string, string> labels = 4;
    repeated string groups = 5;
}
```

#### 3.2 配置

```yaml
agent:
  id: "agent-web-001"
  labels:
    env: "production"
    region: "us-east-1"
    team: "platform"
    role: "web-server"
  groups:
    - "web-servers"
    - "us-east-infra"
```

#### 3.3 平台侧能力

- 按标签组合筛选 Agent（`env=prod AND role=web`）
- 按组批量推送配置/命令
- 版本分布统计
- 标签不一致告警

### 4. 自动更新

#### 4.1 更新流程

```
平台推送 AgentUpdate → 下载新二进制 → 校验签名+SHA256
→ 替换旧二进制 → 重启服务 → 健康检查通过 → 确认更新
→ 健康检查失败 → 自动回滚到旧二进制
```

#### 4.2 组件

```go
// internal/updater/updater.go
type Updater struct {
    currentPath string            // /usr/local/bin/opsagent
    backupPath  string            // /usr/local/bin/opsagent.bak
    downloadDir string            // /tmp/opsagent/update/
    publicKey   ed25519.PublicKey
    grpcClient  GRPCClient
    logger      zerolog.Logger
}

type UpdateRequest struct {
    Version      string `json:"version"`
    DownloadURL  string `json:"download_url"`
    SHA256       string `json:"sha256"`
    Signature    string `json:"signature"`
    ForceRestart bool   `json:"force_restart"`
}

func (u *Updater) Apply(ctx context.Context, req UpdateRequest) error {
    // 1. 下载新二进制到临时目录
    // 2. 验证 SHA-256 校验和
    // 3. 验证 Ed25519 签名
    // 4. 备份当前二进制
    // 5. 替换二进制（原子 rename）
    // 6. 重启服务（systemctl restart opsagent）
    // 7. 启动后健康检查
    // 8. 失败则回滚
}
```

#### 4.3 Proto 消息

```protobuf
message AgentUpdate {
    string version = 1;
    string download_url = 2;
    string sha256 = 3;
    bytes signature = 4;
    bool force_restart = 5;
}

message AgentUpdateAck {
    string agent_id = 1;
    string from_version = 2;
    string to_version = 3;
    string status = 4;         // downloading, verifying, applied, rolled_back, failed
    string error = 5;
}
```

#### 4.4 安全要求

- Ed25519 签名验证（防止二进制篡改）
- SHA-256 校验和（防止传输错误）
- 仅从平台签名的 URL 下载
- 平台控制分阶段推送（1% → 5% → 25% → 100%）
- 更新失败自动回滚 + 平台通知

#### 4.5 环境适配

- **裸机/VM**：通过 `systemctl restart opsagent` 重启服务，A/B 二进制替换
- **容器/K8s**：Agent 不自行更新二进制；平台通过更新镜像 tag 触发 DaemonSet 滚动更新，Agent 仅接收 `AgentUpdate` 消息并上报当前版本，由 Operator/Helm 控制实际升级
- **判断逻辑**：Agent 启动时检测 `/.dockerenv` 或 `/proc/1/cgroup` 判断是否运行在容器中，据此选择更新策略

#### 4.6 回滚机制

```go
func (u *Updater) Rollback() error {
    // 1. 检查备份文件是否存在
    // 2. 原子 rename 备份 → 当前
    // 3. 重启服务
    // 4. 验证健康检查
}
```

## 关键文件

| 文件 | 操作 |
|------|------|
| `internal/discovery/` | **新建** — 服务自发现引擎 |
| `internal/discovery/systemd.go` | **新建** — systemd 发现层 |
| `internal/discovery/proc.go` | **新建** — /proc 发现层 |
| `internal/discovery/container.go` | **新建** — 容器发现层 |
| `internal/discovery/metadata.go` | **新建** — 云元数据发现层 |
| `internal/discovery/discovery_test.go` | **新建** |
| `internal/templates/` | **新建** — 配置模板库 |
| `internal/templates/templates/*.yaml` | **新建** — 模板文件 |
| `internal/templates/loader.go` | **新建** — 模板加载器 |
| `internal/updater/` | **新建** — 自动更新引擎 |
| `internal/updater/updater.go` | **新建** — A/B 替换逻辑 |
| `internal/updater/updater_test.go` | **新建** |
| `internal/app/agent.go` | **修改** — 集成 DiscoveryService |
| `internal/app/interfaces.go` | **修改** — 新增 DiscoveryService 接口 |
| `internal/app/commands.go` | **修改** — 新增 templates 子命令 |
| `internal/config/config.go` | **修改** — 新增 discovery/updater 配置段 |
| `proto/agent.proto` | **修改** — 新增 ServiceDiscovery/AgentUpdate 消息 |

## 测试要求

- Discovery：systemd 单元枚举、端口映射、容器发现、元数据获取
- 模板加载：变量替换、模板应用、CLI 命令
- 标签：注册消息携带标签、心跳更新标签
- 自动更新：下载、签名验证、替换、回滚

## 验证方式

```bash
# 单元测试
go test -race ./internal/discovery/...
go test -race ./internal/templates/...
go test -race ./internal/updater/...

# CLI 验证
opsagent templates list
opsagent templates show nginx
opsagent templates suggest

# 集成测试
go test -race -tags=integration ./internal/integration/ -run TestDiscovery
go test -race -tags=integration ./internal/integration/ -run TestAutoUpdate
```
