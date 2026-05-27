# Platform Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create production-grade Helm Chart for DaemonSet deployment, OpenAPI 3.1 spec for HTTP endpoints, and service mesh integration (Cilium P0, Istio Ambient P1, Linkerd P2).

**Architecture:** New `deploy/helm/opsagent/` directory with complete Helm Chart including DaemonSet, RBAC, PriorityClass, PDB, NetworkPolicy, and CiliumNetworkPolicy templates. New `api/openapi.yaml` with OpenAPI 3.1.0 spec. Makefile additions for validation.

**Tech Stack:** Helm 3.x, YAML templates, OpenAPI 3.1.0, Cilium CNI, go embed for Swagger UI (optional).

---

## File Structure

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
│   ├── ciliumnetworkpolicy.yaml
│   ├── NOTES.txt
│   └── tests/
│       └── test-connection.yaml
└── README.md

api/
└── openapi.yaml

Makefile                         # Modified: add helm-lint, openapi-validate targets
```

---

## Task 1: Helm Chart Scaffolding

**Files:**
- Create: `deploy/helm/opsagent/Chart.yaml`
- Create: `deploy/helm/opsagent/values.yaml`
- Create: `deploy/helm/opsagent/templates/_helpers.tpl`

- [ ] **Step 1: Create Chart.yaml**

```yaml
# deploy/helm/opsagent/Chart.yaml
apiVersion: v2
name: opsagent
description: OpsAgent - Host-side agent for OpsPilot platform
type: application
version: 0.1.0
appVersion: "1.0.0"
keywords:
  - agent
  - monitoring
  - observability
  - ops
maintainers:
  - name: opsagent-team
```

- [ ] **Step 2: Create values.yaml**

```yaml
# deploy/helm/opsagent/values.yaml
image:
  repository: ghcr.io/cy77cc/opsagent
  tag: "latest"
  pullPolicy: IfNotPresent

agent:
  id: ""
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

mesh:
  enabled: false
  type: "cilium"
  cilium:
    networkPolicy:
      enabled: true
      egress:
        - toEndpoints:
            - matchLabels:
                app: opsplatform
          toPorts:
            - ports:
                - port: "443"
                  protocol: TCP
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
  istio:
    ambient:
      enabled: true
  linkerd:
    inject: false
```

- [ ] **Step 3: Create _helpers.tpl**

```yaml
# deploy/helm/opsagent/templates/_helpers.tpl
{{/*
Expand the name of the chart.
*/}}
{{- define "opsagent.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "opsagent.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "opsagent.labels" -}}
helm.sh/chart: {{ include "opsagent.name" . }}-{{ .Chart.Version }}
app.kubernetes.io/name: {{ include "opsagent.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "opsagent.selectorLabels" -}}
app: {{ include "opsagent.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "opsagent.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "opsagent.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
```

- [ ] **Step 4: Run helm lint**

Run: `helm lint deploy/helm/opsagent/`
Expected: PASS (may warn about missing templates, which is expected)

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/opsagent/Chart.yaml deploy/helm/opsagent/values.yaml deploy/helm/opsagent/templates/_helpers.tpl
git commit -m "feat: add Helm Chart scaffolding with values.yaml and helpers"
```

---

## Task 2: DaemonSet Template

**Files:**
- Create: `deploy/helm/opsagent/templates/daemonset.yaml`

- [ ] **Step 1: Create DaemonSet template**

```yaml
# deploy/helm/opsagent/templates/daemonset.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {{ include "opsagent.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
spec:
  selector:
    matchLabels:
      {{- include "opsagent.selectorLabels" . | nindent 6 }}
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  template:
    metadata:
      labels:
        {{- include "opsagent.selectorLabels" . | nindent 8 }}
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
    spec:
      serviceAccountName: {{ include "opsagent.serviceAccountName" . }}
      automountServiceAccountToken: {{ .Values.serviceAccount.automountServiceAccountToken }}
      {{- if .Values.priorityClass.create }}
      priorityClassName: {{ .Values.priorityClass.name }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: opsagent
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          args:
            - "run"
            - "--config"
            - "/etc/opsagent/config.yaml"
          env:
            - name: HOSTNAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          ports:
            - name: http
              containerPort: 18080
              protocol: TCP
          {{- with .Values.livenessProbe }}
          livenessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.readinessProbe }}
          readinessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
          securityContext:
            {{- toYaml .Values.securityContext | nindent 12 }}
          volumeMounts:
            - name: config
              mountPath: /etc/opsagent
              readOnly: true
            - name: tmp
              mountPath: /tmp
            {{- if .Values.grpc.mtls.enabled }}
            - name: tls
              mountPath: /etc/opsagent/tls
              readOnly: true
            {{- end }}
      volumes:
        - name: config
          configMap:
            name: {{ include "opsagent.fullname" . }}
        - name: tmp
          emptyDir: {}
        {{- if .Values.grpc.mtls.enabled }}
        - name: tls
          secret:
            secretName: {{ include "opsagent.fullname" . }}-tls
        {{- end }}
```

- [ ] **Step 2: Test template rendering**

Run: `helm template opsagent deploy/helm/opsagent/ --dry-run`
Expected: Valid YAML output with DaemonSet

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/opsagent/templates/daemonset.yaml
git commit -m "feat: add DaemonSet template with security hardening"
```

---

## Task 3: RBAC Templates

**Files:**
- Create: `deploy/helm/opsagent/templates/serviceaccount.yaml`
- Create: `deploy/helm/opsagent/templates/clusterrole.yaml`
- Create: `deploy/helm/opsagent/templates/clusterrolebinding.yaml`

- [ ] **Step 1: Create ServiceAccount**

```yaml
# deploy/helm/opsagent/templates/serviceaccount.yaml
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "opsagent.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
{{- end }}
```

- [ ] **Step 2: Create ClusterRole**

```yaml
# deploy/helm/opsagent/templates/clusterrole.yaml
{{- if .Values.serviceAccount.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "opsagent.fullname" . }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
rules:
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list"]
{{- end }}
```

- [ ] **Step 3: Create ClusterRoleBinding**

```yaml
# deploy/helm/opsagent/templates/clusterrolebinding.yaml
{{- if .Values.serviceAccount.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "opsagent.fullname" . }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "opsagent.fullname" . }}
subjects:
  - kind: ServiceAccount
    name: {{ include "opsagent.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
{{- end }}
```

- [ ] **Step 4: Test rendering**

Run: `helm template opsagent deploy/helm/opsagent/ --dry-run | grep -A5 "kind: ClusterRole"`
Expected: Shows ClusterRole with minimal permissions

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/opsagent/templates/serviceaccount.yaml deploy/helm/opsagent/templates/clusterrole.yaml deploy/helm/opsagent/templates/clusterrolebinding.yaml
git commit -m "feat: add RBAC templates with minimal permissions"
```

---

## Task 4: ConfigMap, PriorityClass, PDB Templates

**Files:**
- Create: `deploy/helm/opsagent/templates/configmap.yaml`
- Create: `deploy/helm/opsagent/templates/priorityclass.yaml`
- Create: `deploy/helm/opsagent/templates/pdb.yaml`

- [ ] **Step 1: Create ConfigMap**

```yaml
# deploy/helm/opsagent/templates/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "opsagent.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
data:
  config.yaml: |
    agent:
      id: "{{ .Values.agent.id }}"
      interval_seconds: {{ .Values.agent.intervalSeconds }}
      labels:
        {{- range $k, $v := .Values.agent.labels }}
        {{ $k }}: "{{ $v }}"
        {{- end }}
      groups:
        {{- range .Values.agent.groups }}
        - "{{ . }}"
        {{- end }}
    server:
      listen_addr: "{{ .Values.server.listenAddr }}"
    grpc:
      server_addr: "{{ .Values.grpc.serverAddr }}"
      enroll_token: "{{ .Values.grpc.enrollToken }}"
      mtls:
        enabled: {{ .Values.grpc.mtls.enabled }}
        cert_file: "{{ .Values.grpc.mtls.certFile }}"
        key_file: "{{ .Values.grpc.mtls.keyFile }}"
        ca_file: "{{ .Values.grpc.mtls.caFile }}"
    auth:
      enabled: {{ .Values.auth.enabled }}
      bearer_token: "{{ .Values.auth.bearerToken }}"
    sandbox:
      enabled: {{ .Values.sandbox.enabled }}
    plugin:
      enabled: {{ .Values.plugin.enabled }}
    plugin_gateway:
      enabled: {{ .Values.pluginGateway.enabled }}
    collector:
      inputs:
        {{- toYaml .Values.collector.inputs | nindent 8 }}
      processors:
        {{- toYaml .Values.collector.processors | nindent 8 }}
      aggregators:
        {{- toYaml .Values.collector.aggregators | nindent 8 }}
      outputs:
        {{- toYaml .Values.collector.outputs | nindent 8 }}
    logging:
      level: "{{ .Values.logging.level }}"
```

- [ ] **Step 2: Create PriorityClass**

```yaml
# deploy/helm/opsagent/templates/priorityclass.yaml
{{- if .Values.priorityClass.create -}}
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: {{ .Values.priorityClass.name }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
value: {{ .Values.priorityClass.value }}
globalDefault: false
description: "Priority class for OpsAgent DaemonSet"
{{- end }}
```

- [ ] **Step 3: Create PDB**

```yaml
# deploy/helm/opsagent/templates/pdb.yaml
{{- if .Values.podDisruptionBudget.enabled -}}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "opsagent.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
spec:
  maxUnavailable: {{ .Values.podDisruptionBudget.maxUnavailable }}
  selector:
    matchLabels:
      {{- include "opsagent.selectorLabels" . | nindent 6 }}
{{- end }}
```

- [ ] **Step 4: Test rendering**

Run: `helm template opsagent deploy/helm/opsagent/ --dry-run | grep -c "kind:"`
Expected: Shows multiple resource kinds

- [ ] **Step 5: Commit**

```bash
git add deploy/helm/opsagent/templates/configmap.yaml deploy/helm/opsagent/templates/priorityclass.yaml deploy/helm/opsagent/templates/pdb.yaml
git commit -m "feat: add ConfigMap, PriorityClass, and PDB templates"
```

---

## Task 5: NetworkPolicy and CiliumNetworkPolicy

**Files:**
- Create: `deploy/helm/opsagent/templates/networkpolicy.yaml`
- Create: `deploy/helm/opsagent/templates/ciliumnetworkpolicy.yaml`

- [ ] **Step 1: Create NetworkPolicy**

```yaml
# deploy/helm/opsagent/templates/networkpolicy.yaml
{{- if .Values.networkPolicy.enabled -}}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "opsagent.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      {{- include "opsagent.selectorLabels" . | nindent 6 }}
  policyTypes:
    - Egress
  egress:
    {{- toYaml .Values.networkPolicy.egress | nindent 4 }}
{{- end }}
```

- [ ] **Step 2: Create CiliumNetworkPolicy**

```yaml
# deploy/helm/opsagent/templates/ciliumnetworkpolicy.yaml
{{- if and .Values.mesh.enabled (eq .Values.mesh.type "cilium") .Values.mesh.cilium.networkPolicy.enabled -}}
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: {{ include "opsagent.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
spec:
  endpointSelector:
    matchLabels:
      {{- include "opsagent.selectorLabels" . | nindent 6 }}
  egress:
    {{- toYaml .Values.mesh.cilium.networkPolicy.egress | nindent 4 }}
{{- end }}
```

- [ ] **Step 3: Test Cilium rendering**

Run: `helm template opsagent deploy/helm/opsagent/ --set mesh.enabled=true --set mesh.type=cilium --dry-run | grep -A20 "CiliumNetworkPolicy"`
Expected: Shows CiliumNetworkPolicy with egress rules

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/opsagent/templates/networkpolicy.yaml deploy/helm/opsagent/templates/ciliumnetworkpolicy.yaml
git commit -m "feat: add NetworkPolicy and CiliumNetworkPolicy templates"
```

---

## Task 6: NOTES.txt and Test Template

**Files:**
- Create: `deploy/helm/opsagent/templates/NOTES.txt`
- Create: `deploy/helm/opsagent/templates/tests/test-connection.yaml`

- [ ] **Step 1: Create NOTES.txt**

```
# deploy/helm/opsagent/templates/NOTES.txt
OpsAgent has been deployed as a DaemonSet.

To check status:
  kubectl get daemonset {{ include "opsagent.fullname" . }} -n {{ .Release.Namespace }}

To view logs:
  kubectl logs -l app={{ include "opsagent.fullname" . }} -n {{ .Release.Namespace }} -f

To verify health:
  kubectl exec -it $(kubectl get pod -l app={{ include "opsagent.fullname" . }} -n {{ .Release.Namespace }} -o jsonpath='{.items[0].metadata.name}') -n {{ .Release.Namespace }} -- curl -s http://127.0.0.1:18080/healthz
```

- [ ] **Step 2: Create test template**

```yaml
# deploy/helm/opsagent/templates/tests/test-connection.yaml
apiVersion: v1
kind: Pod
metadata:
  name: "{{ include "opsagent.fullname" . }}-test-connection"
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "opsagent.labels" . | nindent 4 }}
  annotations:
    "helm.sh/hook": test
spec:
  containers:
    - name: wget
      image: busybox
      command: ['wget']
      args: ['{{ include "opsagent.fullname" . }}:18080/healthz']
  restartPolicy: Never
```

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/opsagent/templates/NOTES.txt deploy/helm/opsagent/templates/tests/
git commit -m "feat: add Helm NOTES.txt and test template"
```

---

## Task 7: OpenAPI Specification

**Files:**
- Create: `api/openapi.yaml`

- [ ] **Step 1: Create OpenAPI 3.1 spec**

```yaml
# api/openapi.yaml
openapi: "3.1.0"
info:
  title: OpsAgent HTTP API
  description: HTTP API for OpsAgent health checks, task execution, and management
  version: "1.0.0"
  license:
    name: MIT
servers:
  - url: http://127.0.0.1:18080
    description: Local agent

paths:
  /healthz:
    get:
      operationId: getHealth
      summary: Health check
      tags: [Health]
      responses:
        "200":
          description: Healthy
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/HealthResponse"

  /readyz:
    get:
      operationId: getReady
      summary: Readiness probe
      tags: [Health]
      responses:
        "200":
          description: Ready
        "503":
          description: Not ready

  /metrics:
    get:
      operationId: getMetrics
      summary: Prometheus metrics
      tags: [Metrics]
      responses:
        "200":
          description: Prometheus text format
          content:
            text/plain:
              schema:
                type: string

  /api/v1/exec:
    post:
      operationId: executeCommand
      summary: Execute a command
      tags: [Execution]
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/ExecRequest"
      responses:
        "200":
          description: Command result
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ApiResponse"
        "401":
          $ref: "#/components/responses/Unauthorized"

  /api/v1/tasks:
    post:
      operationId: submitTask
      summary: Submit a task
      tags: [Tasks]
      security:
        - bearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/TaskRequest"
      responses:
        "200":
          description: Task accepted
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ApiResponse"

  /api/v1/config:
    get:
      operationId: getConfig
      summary: Get current configuration (secrets masked)
      tags: [Management]
      security:
        - bearerAuth: []
      responses:
        "200":
          description: Current config
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ApiResponse"

  /api/v1/plugins:
    get:
      operationId: listPlugins
      summary: List loaded plugins
      tags: [Management]
      security:
        - bearerAuth: []
      responses:
        "200":
          description: Plugin list
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ApiResponse"

  /api/v1/health/detailed:
    get:
      operationId: getDetailedHealth
      summary: Detailed subsystem health
      tags: [Health]
      responses:
        "200":
          description: Detailed health
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ApiResponse"

components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer

  schemas:
    HealthResponse:
      type: object
      properties:
        status:
          type: string
          enum: [healthy, degraded, unhealthy]
        version:
          type: string
        uptime:
          type: string
        subsystems:
          type: object
          additionalProperties:
            type: object
            properties:
              status:
                type: string
              details:
                type: object

    ExecRequest:
      type: object
      required: [command]
      properties:
        command:
          type: string
        args:
          type: array
          items:
            type: string
        timeout_seconds:
          type: integer
          default: 30

    TaskRequest:
      type: object
      required: [type]
      properties:
        type:
          type: string
        payload:
          type: object
        timeout_seconds:
          type: integer

    ApiResponse:
      type: object
      properties:
        success:
          type: boolean
        data:
          type: object
          nullable: true
        error:
          type: string
          nullable: true

  responses:
    Unauthorized:
      description: Authentication required
```

- [ ] **Step 2: Validate OpenAPI spec**

Run: `npx @redocly/cli lint api/openapi.yaml` or `npx swagger-cli validate api/openapi.yaml`
Expected: Valid spec

- [ ] **Step 3: Commit**

```bash
git add api/openapi.yaml
git commit -m "feat: add OpenAPI 3.1 spec for HTTP endpoints"
```

---

## Task 8: Makefile and README Updates

**Files:**
- Modify: `Makefile`
- Create: `deploy/helm/opsagent/README.md`

- [ ] **Step 1: Add Makefile targets**

```makefile
# Helm targets
.PHONY: helm-lint helm-template helm-install

helm-lint:
	helm lint deploy/helm/opsagent/

helm-template:
	helm template opsagent deploy/helm/opsagent/ --dry-run

helm-install:
	helm install opsagent deploy/helm/opsagent/ --namespace opsagent --create-namespace

# OpenAPI targets
.PHONY: openapi-validate

openapi-validate:
	npx @redocly/cli lint api/openapi.yaml
```

- [ ] **Step 2: Create Helm README**

```markdown
# deploy/helm/opsagent/README.md
# OpsAgent Helm Chart

## Installation

```bash
helm install opsagent deploy/helm/opsagent/ \
  --namespace opsagent --create-namespace \
  --set grpc.serverAddr=platform.example.com:443 \
  --set grpc.enrollToken=YOUR_TOKEN
```

## Configuration

See `values.yaml` for all configurable parameters.

## Service Mesh

### Cilium

```bash
helm install opsagent deploy/helm/opsagent/ \
  --set mesh.enabled=true \
  --set mesh.type=cilium
```

### Istio Ambient

```bash
helm install opsagent deploy/helm/opsagent/ \
  --set mesh.enabled=true \
  --set mesh.type=istio
```
```

- [ ] **Step 3: Run helm lint**

Run: `make helm-lint`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add Makefile deploy/helm/opsagent/README.md
git commit -m "feat: add helm-lint and openapi-validate Makefile targets"
```

---

## Task 9: Full Template Validation

**Files:** None (validation only)

- [ ] **Step 1: Validate default values**

Run: `helm template opsagent deploy/helm/opsagent/ --dry-run`
Expected: Clean YAML output with DaemonSet, SA, ClusterRole, CRB, ConfigMap, PriorityClass, PDB

- [ ] **Step 2: Validate with Cilium mesh**

Run: `helm template opsagent deploy/helm/opsagent/ --set mesh.enabled=true --set mesh.type=cilium --dry-run`
Expected: Includes CiliumNetworkPolicy

- [ ] **Step 3: Validate with custom values**

Run: `helm template opsagent deploy/helm/opsagent/ --set agent.intervalSeconds=30 --set logging.level=debug --dry-run`
Expected: Values override correctly in ConfigMap

- [ ] **Step 4: Lint**

Run: `make helm-lint`
Expected: PASS

- [ ] **Step 5: OpenAPI validation**

Run: `make openapi-validate`
Expected: PASS

- [ ] **Step 6: Final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: validation fixes for Helm Chart and OpenAPI spec"
```
