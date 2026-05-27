# Spec: 平台集成深化

> 日期: 2026-05-27
> 状态: Approved
> 作者: AI Assistant + User

## Context

OpsAgent 已完整实现所有核心功能，但缺少 Kubernetes 生产部署支持和标准 API 规范。本 spec 定义 Helm Chart、OpenAPI Spec、Kubernetes Operator（未来）、服务网格集成四个平台集成功能。

## 目标

1. Helm Chart：生产级 DaemonSet 部署，完整安全加固
2. OpenAPI Spec：为现有 HTTP 端点生成 OpenAPI 3.1 规范
3. Kubernetes Operator（未来阶段）：CRD 管理 Agent 部署和配置
4. 服务网格集成：支持 Cilium（优先）、Istio Ambient、Linkerd

## 依赖

- 现有 HTTP Server（`internal/server/`）
- 现有配置系统（`internal/config/`）
- Kubernetes 集群环境
- Helm 3.x

## 设计

### 1. Helm Chart

#### 1.1 目录结构

```
deploy/helm/opsagent/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── _helpers.tpl
│   ├── daemonset.yaml
│   ├── serviceaccount.yaml
│   ├── clusterrole.yaml
│   ├── clusterrolebinding.yaml
│   ├── configmap.yaml
│   ├── priorityclass.yaml
│   ├── pdb.yaml
│   ├── networkpolicy.yaml
│   ├── NOTES.txt
│   └── tests/
│       └── test-connection.yaml
└── README.md
```

#### 1.2 values.yaml

```yaml
image:
  repository: ghcr.io/cy77cc/opsagent
  tag: "latest"
  pullPolicy: IfNotPresent

agent:
  id: ""                     # 留空使用 hostname
  intervalSeconds: 10
  labels: {}
  groups: []

grpc:
  serverAddr: "platform.example.com:443"
  enrollToken: ""
  mtls:
    enabled: false
    certFile: "/etc/opsagent/tls/tls.crt"
    keyFile: "/etc/opsagent/tls/tls.key"
    caFile: "/etc/opsagent/tls/ca.crt"

server:
  listenAddr: "127.0.0.1:18080"

collector:
  inputs:
    - type: cpu
      config: { per_cpu: false }
    - type: memory
      config: {}
    - type: disk
      config: {}
    - type: net
      config: {}
    - type: load
      config: {}
  processors: []
  aggregators: []
  outputs: []

auth:
  enabled: true
  bearerToken: ""

sandbox:
  enabled: false

plugin:
  enabled: false
pluginGateway:
  enabled: false

securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

tolerations:
  - key: "node-role.kubernetes.io/control-plane"
    operator: "Exists"
    effect: "NoSchedule"
  - key: "node-role.kubernetes.io/master"
    operator: "Exists"
    effect: "NoSchedule"

priorityClass:
  create: true
  name: "opsagent-critical"
  value: 1000000000

podDisruptionBudget:
  enabled: true
  maxUnavailable: 1

networkPolicy:
  enabled: false
  egress:
    - to:
        - ipBlock:
            cidr: "0.0.0.0/0"
      ports:
        - port: 443
          protocol: TCP

serviceAccount:
  create: true
  automountServiceAccountToken: false

livenessProbe:
  httpGet:
    path: /healthz
    port: 18080
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /readyz
    port: 18080
  initialDelaySeconds: 5
  periodSeconds: 10

logging:
  level: "info"

# Mesh 集成
mesh:
  enabled: false
  type: "cilium"             # cilium | istio | linkerd
```

#### 1.3 DaemonSet 关键设计

- `updateStrategy: RollingUpdate` + `maxUnavailable: 1`
- `priorityClassName: opsagent-critical`
- 安全上下文：`runAsNonRoot`、`readOnlyRootFilesystem`、drop all capabilities
- 资源限制：CPU 500m / Memory 512Mi（可配置）
- 健康检查：liveness `/healthz`、readiness `/readyz`
- 不使用 `hostPID`、`hostNetwork`（除非显式启用）

#### 1.4 RBAC

最小权限原则：
- ClusterRole：仅 `get/list/watch` nodes（用于主机信息收集）
- ClusterRoleBinding：绑定到 opsagent ServiceAccount
- 通过 `rbac.create` toggle 控制是否创建

### 2. OpenAPI Spec

#### 2.1 规范文件

`api/openapi.yaml`，OpenAPI 3.1.0 格式。

#### 2.2 覆盖端点

| 端点 | 方法 | 操作 |
|------|------|------|
| `/healthz` | GET | getHealth |
| `/readyz` | GET | getReady |
| `/metrics` | GET | getMetrics |
| `/api/v1/exec` | POST | executeCommand |
| `/api/v1/tasks` | POST | submitTask |
| `/api/v1/config` | GET | getConfig |
| `/api/v1/plugins` | GET | listPlugins |
| `/api/v1/health/detailed` | GET | getDetailedHealth |
| `/ui/` | GET | getUI |

#### 2.3 Schema 定义

主要 Schema：

- `HealthResponse`：status (healthy/degraded/unhealthy)、version、uptime、subsystems
- `ExecRequest`：command、args、timeout_seconds
- `ExecResult`：exit_code、stdout、stderr、duration_ms
- `TaskRequest`：type、payload、timeout_seconds
- `TaskResult`：task_id、status、result
- `PluginInfo`：name、version、status、task_types
- `DetailedHealthResponse`：status、subsystems 详细状态

#### 2.4 集成方式

- Makefile 新增 `make openapi-validate`（使用 `swagger-cli` 验证）
- CI 添加 OpenAPI 规范校验步骤
- 可选：嵌入 Swagger UI 到 `/ui/docs`

### 3. Kubernetes Operator（未来阶段）

#### 3.1 CRD 设计

**OpsAgentConfig CRD**：

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: opsagentconfigs.opsagent.io
spec:
  group: opsagent.io
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                image:
                  type: string
                intervalSeconds:
                  type: integer
                inputs:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                      config:
                        type: object
                labels:
                  type: object
                  additionalProperties:
                    type: string
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: [Pending, Deploying, Ready, Degraded, Failed]
                nodeStatuses:
                  type: array
                  items:
                    type: object
                    properties:
                      nodeName:
                        type: string
                      agentStatus:
                        type: string
                      lastHeartbeat:
                        type: string
                observedGeneration:
                  type: integer
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                      status:
                        type: string
                      lastTransitionTime:
                        type: string
                      reason:
                        type: string
                      message:
                        type: string
      subresources:
        status: {}
```

#### 3.2 Operator 职责

1. 监听 `OpsAgentConfig` CR 变更
2. 自动生成/更新 DaemonSet
3. 收集各节点 Agent 健康状态
4. 更新 `OpsAgentConfig.status`
5. 发送 Kubernetes Events

#### 3.3 条件类型

- `Ready`：所有节点 Agent 运行正常
- `Degraded`：部分节点 Agent 异常
- `Reconciled`：Operator 已处理最新 spec

#### 3.4 工具选型

- Kubebuilder 脚手架
- controller-runtime 框架
- Server-Side Apply (SSA) 更新 status

### 4. 服务网格集成

#### 4.1 支持矩阵

| Mesh | 部署模式 | mTLS | 网络策略 | 优先级 |
|------|----------|------|----------|--------|
| **Cilium** | eBPF 内核级 | 自动生成（无 sidecar） | L3/L4/L7 CiliumNetworkPolicy | P0 |
| Istio | Ambient mesh（ztunnel） | ztunnel L4 mTLS | AuthorizationPolicy | P1 |
| Linkerd | linkerd2-proxy | 自动 mTLS | Server/ServerAuthorization | P2 |

#### 4.2 Cilium 集成

**设计原则**：Cilium 作为一等公民支持，eBPF 零开销。

```yaml
mesh:
  enabled: false
  type: "cilium"
  cilium:
    # CiliumNetworkPolicy 自动生成
    networkPolicy:
      enabled: true
      egress:
        # 平台 gRPC 通信
        - toEndpoints:
            - matchLabels:
                app: opsplatform
          toPorts:
            - ports:
                - port: "443"
                  protocol: TCP
        # DNS
        - toEndpoints:
            - matchLabels:
                k8s:io.kubernetes.pod.namespace: kube-system
                k8s-app: kube-dns
          toPorts:
            - ports:
                - port: "53"
                  protocol: UDP
                - port: "53"
                  protocol: TCP
    # Hubble 可观测性
    hubble:
      enabled: true
      # Agent 流量自动被 Hubble 采集
    # Cilium Service Mesh（L7 策略，eBPF 实现）
    serviceMesh:
      enabled: false
    # 带宽管理
    bandwidthManager:
      enabled: false
```

**CiliumNetworkPolicy 模板**：

```yaml
{{- if and .Values.mesh.enabled (eq .Values.mesh.type "cilium") }}
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: {{ include "opsagent.fullname" . }}
  namespace: {{ .Release.Namespace }}
spec:
  endpointSelector:
    matchLabels:
      app: {{ include "opsagent.fullname" . }}
  egress:
    {{- toYaml .Values.mesh.cilium.networkPolicy.egress | nindent 4 }}
{{- end }}
```

#### 4.3 Istio Ambient 集成

```yaml
mesh:
  enabled: false
  type: "istio"
  istio:
    ambient:
      enabled: true
      # ztunnel L4 mTLS，无需 sidecar
    networkPolicy:
      enabled: true
      egress:
        - to:
            - ipBlock:
                cidr: "0.0.0.0/0"
          ports:
            - port: 443
              protocol: TCP
```

#### 4.4 Linkerd 集成

```yaml
mesh:
  enabled: false
  type: "linkerd"
  linkerd:
    inject: false
    # Agent DaemonSet 不注入 sidecar
    mtls:
      useLinkerdIdentity: true
      certPath: "/var/run/linkerd/identity/..."
```

#### 4.5 选择建议

| 场景 | 推荐 | 理由 |
|------|------|------|
| 新建 K8s 集群 | Cilium | eBPF 原生，零开销，Hubble 可观测性 |
| 已有 Istio 集群 | Istio ambient | 无 sidecar 开销，复用现有基础设施 |
| 轻量级部署 | Linkerd | 最小资源占用，简单运维 |
| 非 K8s 环境 | 不启用 mesh | Agent 自带 mTLS + PSK |

## 关键文件

| 文件 | 操作 |
|------|------|
| `deploy/helm/opsagent/` | **新建** — Helm Chart |
| `deploy/helm/opsagent/values.yaml` | **新建** — 默认配置 |
| `deploy/helm/opsagent/templates/daemonset.yaml` | **新建** |
| `deploy/helm/opsagent/templates/configmap.yaml` | **新建** |
| `deploy/helm/opsagent/templates/serviceaccount.yaml` | **新建** |
| `deploy/helm/opsagent/templates/clusterrole.yaml` | **新建** |
| `deploy/helm/opsagent/templates/clusterrolebinding.yaml` | **新建** |
| `deploy/helm/opsagent/templates/priorityclass.yaml` | **新建** |
| `deploy/helm/opsagent/templates/pdb.yaml` | **新建** |
| `deploy/helm/opsagent/templates/networkpolicy.yaml` | **新建** |
| `deploy/helm/opsagent/templates/_helpers.tpl` | **新建** |
| `deploy/helm/opsagent/Chart.yaml` | **新建** |
| `deploy/helm/opsagent/README.md` | **新建** |
| `api/openapi.yaml` | **新建** — OpenAPI 3.1 规范 |
| `Makefile` | **修改** — 新增 openapi-validate、helm 目标 |

## 测试要求

- Helm Chart：`helm template` 渲染正确、`helm lint` 通过、values 覆盖生效
- DaemonSet：安全上下文、资源限制、容忍度、优先级
- RBAC：最小权限验证
- OpenAPI：规范校验通过、与实际端点一致
- Cilium 策略：CiliumNetworkPolicy 生成正确

## 验证方式

```bash
# Helm Chart 验证
helm lint deploy/helm/opsagent/
helm template opsagent deploy/helm/opsagent/ --dry-run
helm template opsagent deploy/helm/opsagent/ --set mesh.type=cilium --dry-run

# OpenAPI 验证
make openapi-validate

# 集成测试（需要 K8s 集群）
helm install opsagent deploy/helm/opsagent/ --dry-run
kubectl apply -f deploy/helm/opsagent/templates/ --dry-run=server
```
