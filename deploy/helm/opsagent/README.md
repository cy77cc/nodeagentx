# OpsAgent Helm Chart

Helm chart for deploying OpsAgent as a DaemonSet on every node in a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.10+

## Installation

```bash
# Install from local chart
helm install opsagent deploy/helm/opsagent/ --namespace opsagent --create-namespace

# Install with custom values
helm install opsagent deploy/helm/opsagent/ \
  --namespace opsagent --create-namespace \
  -f my-values.yaml

# Dry-run to preview rendered manifests
helm template opsagent deploy/helm/opsagent/ --dry-run

# Lint the chart
helm lint deploy/helm/opsagent/
```

## Upgrading

```bash
helm upgrade opsagent deploy/helm/opsagent/ --namespace opsagent
```

## Uninstalling

```bash
helm uninstall opsagent --namespace opsagent
```

## Configuration Reference

### Image

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `ghcr.io/cy77cc/opsagent` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `imagePullSecrets` | List of pull secrets | `[]` |

### Deployment

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas (unused for DaemonSet) | `1` |
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full release name | `""` |
| `nodeSelector` | Node selector labels | `{}` |
| `tolerations` | Pod tolerations | `[{operator: Exists}]` |
| `affinity` | Pod affinity rules | `{}` |

### Service Account

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create a service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Override service account name | `""` |

### Security

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext.runAsNonRoot` | Run pod as non-root | `false` |
| `podSecurityContext.runAsUser` | User ID to run as | `0` |
| `securityContext.privileged` | Run in privileged mode | `true` |
| `securityContext.capabilities.add` | Linux capabilities | `[SYS_ADMIN, NET_ADMIN, SYS_PTRACE]` |

### Service

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | HTTP service port | `18080` |

### Resources

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `256Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### Probes

| Parameter | Description | Default |
|-----------|-------------|---------|
| `livenessProbe` | Liveness probe configuration | HTTP GET `/healthz` |
| `readinessProbe` | Readiness probe configuration | HTTP GET `/readyz` |

### PriorityClass

| Parameter | Description | Default |
|-----------|-------------|---------|
| `priorityClass.create` | Create a PriorityClass | `false` |
| `priorityClass.value` | Priority value | `1000000` |
| `priorityClass.globalDefault` | Set as cluster default | `false` |

### PodDisruptionBudget

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podDisruptionBudget.enabled` | Enable PDB | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods | `1` |

### NetworkPolicy

| Parameter | Description | Default |
|-----------|-------------|---------|
| `networkPolicy.enabled` | Enable NetworkPolicy | `false` |
| `networkPolicy.ingress` | Additional ingress rules | `[]` |
| `networkPolicy.egress` | Custom egress rules (replaces allow-all) | `[]` |

### Agent Configuration (`config.*`)

These values are rendered into the agent ConfigMap. See the default `values.yaml` for the full list of nested keys under `config.agent`, `config.server`, `config.executor`, `config.reporter`, `config.auth`, `config.prometheus`, `config.grpc`, `config.collector`, `config.sandbox`, and `config.plugin`.

Key groups:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.agent.interval_seconds` | Metric collection interval | `10` |
| `config.server.listen_addr` | HTTP listen address | `0.0.0.0:18080` |
| `config.executor.timeout_seconds` | Command execution timeout | `10` |
| `config.executor.allowed_commands` | Whitelisted commands | `[uptime, df, free, hostname]` |
| `config.reporter.mode` | Reporting mode (`stdout` or `http`) | `stdout` |
| `config.auth.enabled` | Enable bearer-token auth | `false` |
| `config.prometheus.enabled` | Enable Prometheus exporter | `true` |
| `config.grpc.server_addr` | gRPC platform address | `platform.example.com:443` |
| `config.sandbox.enabled` | Enable nsjail sandbox | `false` |

## Service Mesh Integration

### Cilium

Enable CiliumNetworkPolicy to enforce L3/L4/L7 traffic rules with Cilium:

```yaml
mesh:
  enabled: true
  type: cilium
  cilium:
    networkPolicy:
      enabled: true
      # Restrict egress to specific CIDRs
      egress:
        - toCIDR:
            - 10.0.0.0/8
          toPorts:
            - ports:
                - port: "443"
                  protocol: TCP
```

The CiliumNetworkPolicy template automatically allows:
- Inbound HTTP traffic on `service.port` from pods with matching labels
- DNS resolution (kube-dns on port 53)
- All egress by default (override with `mesh.cilium.networkPolicy.egress`)

### Istio Ambient

For Istio Ambient Mesh, enable standard NetworkPolicy and rely on Istio's ztunnel for L4 mTLS:

```yaml
mesh:
  enabled: false  # Cilium-specific, leave disabled

networkPolicy:
  enabled: true
  ingress:
    # Allow traffic from Istio ingress gateway
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: istio-system
      ports:
        - port: 18080
          protocol: TCP
  egress:
    # Allow traffic to platform gRPC endpoint
    - to:
        - ipBlock:
            cidr: 10.0.0.0/8
      ports:
        - port: 443
          protocol: TCP
    # Allow Istio control plane
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: istio-system
      ports:
        - port: 15012
          protocol: TCP
        - port: 15014
          protocol: TCP
        - port: 15017
          protocol: TCP
```

Ensure the opsagent namespace is labeled for sidecar injection (if using sidecar mode) or included in the ambient mesh:

```bash
kubectl label namespace opsagent istio.io/dataplane-mode=ambient
```

## Verifying the Deployment

```bash
# Watch pod rollout
kubectl get pods -n opsagent -l app.kubernetes.io/name=opsagent -w

# View agent logs
kubectl logs -n opsagent -l app.kubernetes.io/name=opsagent -f

# Check health endpoint
kubectl port-forward -n opsagent svc/opsagent 18080:18080
curl http://localhost:18080/healthz

# Check readiness
curl http://localhost:18080/readyz
```
