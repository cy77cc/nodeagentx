# Gateway Tunnel Platform Integration Document

## 1. Overview

The Gateway Tunnel feature enables OpsAgent to act as a jump host, allowing the platform to manage internal network hosts that cannot be directly connected. All platform operations on internal hosts are identical to directly connected hosts; the jump host logic is transparent to the platform.

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      OpsPilot Platform (A)                      │
│                                                                 │
│  Host List:                                                     │
│  ├── B    (Agent, gateway type)                                 │
│  ├── C1   (jump_host=B, platform sees as normal host)           │
│  ├── C2   (jump_host=B, platform sees as normal host)           │
│  └── Cn   (jump_host=B, platform sees as normal host)           │
└──────────────┬──────────────────────────────────────────────────┘
               │ gRPC Bidirectional Stream (reuses existing channel)
               │
┌──────────────┴──────────────────────────────────────────────────┐
│                   OpsAgent (Host B - Jump Host)                 │
│                                                                 │
│  Gateway Module:                                                │
│  ├── Tunnel Mode: C has Agent, B transparently forwards gRPC   │
│  └── Proxy Mode: C has no Agent, B executes commands via SSH    │
└──────────────┬──────────────────────────────────────────────────┘
               │ TCP / SSH
               ▼
┌─────────────────────────────────────────────────────────────────┐
│  C1 (has Agent)    │  C2 (no Agent)    │  C3 (has Agent)   │ ... │
│  Agent → B Gateway │  B SSH executes   │  Agent → B Gateway│     │
└─────────────────────────────────────────────────────────────────┘
```

### Two Modes Comparison

| | Tunnel Mode (C has Agent) | Proxy Mode (C has no Agent) |
|---|---|---|
| C's appearance on platform | Normal managed host | Normal managed host |
| Metric collection | C collects itself, reports via B tunnel | B collects and reports on behalf |
| Command execution | Platform sends to C, forwarded via B tunnel | B executes and reports on behalf |
| Connection method | C's Agent → B → A | B → C (SSH) |

---

## 2. Proto Message Definitions

### 2.1 New Message Types

```proto
// --- Gateway Tunnel Messages ---

message TunnelOpen {
  string tunnel_id = 1;          // Tunnel unique identifier
  string agent_id = 2;           // C's Agent ID
  string hostname = 3;           // C's hostname
  string ip = 4;                 // C's IP address
  repeated string capabilities = 5; // C's capability list
}

message TunnelData {
  string tunnel_id = 1;          // Tunnel identifier
  bytes payload = 2;             // Serialized AgentMessage or PlatformMessage
}

message TunnelClose {
  string tunnel_id = 1;          // Tunnel identifier
  string reason = 2;             // Close reason
}

message ProxyHostRegister {
  string host_id = 1;            // Host unique identifier
  string hostname = 2;           // Hostname
  string ip = 3;                 // IP address
  repeated string capabilities = 4; // Capability list
}

message ProxyCommandRequest {
  string host_id = 1;            // Target host ID
  string command = 2;            // Command to execute
  repeated string args = 3;      // Command arguments
  int32 timeout_seconds = 4;     // Timeout
}

message ProxyCommandResponse {
  string host_id = 1;            // Target host ID
  string command = 2;            // Executed command
  int32 exit_code = 3;           // Exit code
  bytes stdout = 4;              // Standard output
  bytes stderr = 5;              // Standard error
  int64 duration_ms = 6;         // Execution duration (milliseconds)
  bool timed_out = 7;            // Whether timed out
}

message ProxyMetricBatch {
  string host_id = 1;            // Target host ID
  repeated Metric metrics = 2;   // Metric data
}
```

### 2.2 AgentMessage Extension

```proto
message AgentMessage {
  oneof payload {
    // Existing messages (1-7)
    AgentRegistration registration = 1;
    Heartbeat heartbeat = 2;
    MetricBatch metrics = 3;
    ExecOutput exec_output = 4;
    ExecResult exec_result = 5;
    Ack ack = 6;
    HealthCheckResult health_check_result = 7;

    // Gateway additions (8-13)
    TunnelOpen tunnel_open = 8;           // B → Platform: tunnel established
    TunnelData tunnel_data = 9;           // B → Platform: tunnel data
    TunnelClose tunnel_close = 10;        // B → Platform: tunnel closed
    ProxyHostRegister proxy_register = 11; // B → Platform: proxy host registration
    ProxyCommandResponse proxy_response = 12; // B → Platform: proxy command result
    ProxyMetricBatch proxy_metrics = 13;  // B → Platform: proxy metric data
  }
}
```

### 2.3 PlatformMessage Extension

```proto
message PlatformMessage {
  oneof payload {
    // Existing messages (1-6)
    ExecuteCommand exec_command = 1;
    ExecuteScript exec_script = 2;
    CancelJob cancel_job = 3;
    ConfigUpdate config_update = 4;
    Ack ack = 5;
    HealthCheckRequest health_check = 6;

    // Gateway additions (7-9)
    TunnelData tunnel_data = 7;           // Platform → B: tunnel data
    TunnelClose tunnel_close = 8;         // Platform → B: close tunnel
    ProxyCommandRequest proxy_command = 9; // Platform → B: proxy command request
  }
}
```

---

## 3. Integration Flow

### 3.1 Tunnel Mode (C has Agent)

#### Flow Diagram

```
C's Agent                 B's Agent (Gateway)           Platform A
    │                           │                           │
    │── TCP Connect ───────────►│                           │
    │   (target: B:18081)       │                           │
    │                           │── TunnelOpen ────────────►│
    │                           │   {tunnel_id, agent_id,   │
    │                           │    hostname, ip}          │
    │                           │                           │
    │                           │◄── Ack ──────────────────│
    │                           │                           │
    │◄══════ TCP Bidirectional ═│◄═══ gRPC Bidirectional ══►│
    │           Bridge          │         Tunnel            │
    │                           │                           │
    │── AgentMessage ──────────►│── TunnelData ────────────►│
    │   (registration)          │   {tunnel_id, payload}    │
    │                           │                           │
    │◄── PlatformMessage ──────│◄── TunnelData ───────────│
    │   (ack)                   │   {tunnel_id, payload}    │
    │                           │                           │
    │◄═══════════════════════════════════════════════════►│
    │         All gRPC messages transparently forwarded via B │
```

#### Platform Processing Logic

1. **Receiving TunnelOpen**:
   - Parse `tunnel_id`, `agent_id`, `hostname`, `ip`
   - Create tunnel session, bind `tunnel_id` to this host
   - Register `agent_id` to host list (marked as connected via gateway)
   - Return Ack

2. **Receiving TunnelData**:
   - Look up the corresponding host by `tunnel_id`
   - Deserialize `payload` as `AgentMessage`
   - Process as normal Agent message (registration, heartbeat, metrics, command results, etc.)

3. **Sending message to C**:
   - Serialize `PlatformMessage` to bytes
   - Construct `TunnelData { tunnel_id, payload }`
   - Send via B's gRPC stream

4. **Receiving TunnelClose**:
   - Clean up tunnel session
   - Mark host as offline (consistent with directly connected host disconnection behavior)

### 3.2 Proxy Mode (C has no Agent)

#### Flow Diagram

```
Platform A                     B's Agent (Gateway)           C (no Agent)
  │                           │                           │
  │                           │── ProxyHostRegister ─────►│ (register to platform)
  │                           │   {host_id, hostname, ip} │
  │                           │                           │
  │◄── Ack ──────────────────│                           │
  │                           │                           │
  │── ProxyCommandRequest ──►│                           │
  │   {host_id, command, args}│                           │
  │                           │── SSH Connect ────────────►│
  │                           │── SSH Execute ────────────►│
  │                           │◄── Command Output ────────│
  │                           │                           │
  │◄── ProxyCommandResponse ─│                           │
  │   {host_id, exit_code,    │                           │
  │    stdout, stderr, ...}   │                           │
```

#### Platform Processing Logic

1. **Receiving ProxyHostRegister**:
   - Parse `host_id`, `hostname`, `ip`
   - Register host to host list (marked as proxy mode)
   - Return Ack

2. **Sending command to proxy host**:
   - Construct `ProxyCommandRequest { host_id, command, args, timeout_seconds }`
   - Send via B's gRPC stream

3. **Receiving ProxyCommandResponse**:
   - Parse `host_id`, `exit_code`, `stdout`, `stderr`, `duration_ms`, `timed_out`
   - Process as normal command result

4. **Receiving ProxyMetricBatch**:
   - Parse `host_id` and `metrics`
   - Process as normal metric data

---

## 4. Platform-Side Changes

### 4.1 Host Registration

A new `jump_host` field is added when adding hosts:

```proto
message RegisterRequest {
  string agent_id = 1;
  string hostname = 2;
  string ip = 3;
  string jump_host = 4;  // New: jump host Agent ID, empty means direct connection
}
```

The platform UI adds a "Jump Host" dropdown option when adding hosts, listing all registered gateway-type Agents.

### 4.2 Connection Routing Table

The platform maintains a routing table:

```go
type HostRoute struct {
    HostID    string
    Direct    bool           // true=direct connection, false=via gateway
    GatewayID string         // Gateway Agent ID (valid when Direct=false)
    TunnelID  string         // Tunnel ID (valid in tunnel mode)
}
```

### 4.3 Message Routing Logic

```go
func (p *Platform) SendMessage(hostID string, msg *PlatformMessage) error {
    route := p.routeTable.Get(hostID)
    if route.Direct {
        return p.sendDirect(hostID, msg)
    }

    // Send via gateway
    if route.TunnelID != "" {
        // Tunnel mode: wrap as TunnelData
        payload, _ := proto.Marshal(msg)
        return p.sendToGateway(route.GatewayID, &PlatformMessage{
            Payload: &PlatformMessage_TunnelData{
                TunnelData: &TunnelData{
                    TunnelId: route.TunnelID,
                    Payload:  payload,
                },
            },
        })
    }

    // Proxy mode: send ProxyCommandRequest directly
    return p.sendToGateway(route.GatewayID, msg)
}
```

### 4.4 Receiving Message Routing

```go
func (p *Platform) HandleAgentMessage(gatewayID string, msg *AgentMessage) error {
    switch payload := msg.Payload.(type) {
    case *AgentMessage_TunnelOpen:
        return p.handleTunnelOpen(gatewayID, payload.TunnelOpen)
    case *AgentMessage_TunnelData:
        return p.handleTunnelData(gatewayID, payload.TunnelData)
    case *AgentMessage_TunnelClose:
        return p.handleTunnelClose(gatewayID, payload.TunnelClose)
    case *AgentMessage_ProxyRegister:
        return p.handleProxyRegister(gatewayID, payload.ProxyRegister)
    case *AgentMessage_ProxyResponse:
        return p.handleProxyResponse(gatewayID, payload.ProxyResponse)
    case *AgentMessage_ProxyMetrics:
        return p.handleProxyMetrics(gatewayID, payload.ProxyMetrics)
    default:
        // Normal message processing
        return p.handleNormalMessage(gatewayID, msg)
    }
}
```

---

## 5. Configuration Examples

### 5.1 B's Agent Configuration (Jump Host)

```yaml
agent:
  id: "gateway-b-001"
  name: "gateway-host-b"

gateway:
  enabled: true
  listen_addr: ":18081"          # Port for C's Agent to connect (tunnel mode)
  max_tunnels: 100               # Max tunnel count
  tunnel_timeout_seconds: 30     # Tunnel establishment timeout
  idle_timeout_seconds: 300      # Idle tunnel reclamation

  hosts:
    # Proxy mode: C has no Agent, B executes via SSH on behalf
    - id: "vm-web-01"
      addr: "192.168.122.100"
      mode: "proxy"
      ssh:
        user: "root"
        key_file: "/etc/opsagent/keys/id_rsa"
        port: 22

    # Tunnel mode: C has Agent, B transparently forwards
    - id: "vm-db-01"
      addr: "192.168.122.101"
      mode: "tunnel"

    # Auto mode: B automatically detects whether C has an Agent
    - id: "vm-app-01"
      addr: "192.168.122.102"
      mode: "auto"
      ssh:
        user: "root"
        key_file: "/etc/opsagent/keys/id_rsa"
        port: 22
```

### 5.2 C's Agent Configuration (Tunnel Mode)

```yaml
agent:
  id: "vm-db-01"
  name: "database-server"

grpc:
  server_addr: "192.168.1.10:18081"  # Points to B's Gateway port
  enroll_token: "your-enroll-token"
  heartbeat_interval_seconds: 15
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
```

---

## 6. Error Handling

| Scenario | Platform Behavior | Handling Method |
|------|----------|----------|
| B disconnects from A | All hosts via B show offline | B automatically rebuilds tunnels after reconnecting to A |
| B disconnects from C | Corresponding host shows offline | Consistent with directly connected host disconnection behavior |
| Tunnel connection timeout | Host connection fails | Platform marks host offline, shows after heartbeat timeout |
| B restarts | Tunnels lost, hosts temporarily offline | C's Agent automatically reconnects to B, triggering tunnel rebuild |
| C has no Agent and SSH unreachable | Host unreachable | B reports host offline status |

---

## 7. Observability

### 7.1 Prometheus Metrics

B's Agent exposes the following metrics:

```prometheus
# Current active tunnel count
opsagent_gateway_tunnels_active

# Tunnel traffic statistics (bytes)
opsagent_gateway_tunnel_bytes_total

# Tunnel error count
opsagent_gateway_tunnel_errors_total

# Proxy mode request count
opsagent_gateway_proxy_requests_total

# Proxy execution latency (seconds)
opsagent_gateway_proxy_latency_seconds
```

### 7.2 Health Check

B's `/healthz` endpoint returns the gateway subsystem status:

```json
{
  "status": "healthy",
  "subsystems": {
    "collector": "healthy",
    "grpc": "healthy",
    "gateway": {
      "status": "running",
      "details": {
        "active_tunnels": 15,
        "max_tunnels": 100
      }
    }
  }
}
```

### 7.3 Audit Log

B's Agent records the following audit events:

```json
{"event_type": "gateway.started", "component": "gateway", "action": "start", "status": "success"}
{"event_type": "gateway.tunnel.close", "component": "gateway", "action": "tunnel_close", "details": {"tunnel_id": "xxx", "reason": "idle_timeout"}}
{"event_type": "gateway.proxy.exec", "component": "gateway", "action": "proxy_command", "details": {"host_id": "vm-web-01", "command": "uptime"}}
```

---

## 8. Security Considerations

1. **Tunnel Authentication**: C connects to B using mTLS + enroll_token; B connects to A using existing mTLS
2. **Tunnel Isolation**: Each tunnel_id is bound to a unique C host; the platform verifies tunnel_id matches the target host
3. **Proxy Mode Security**: SSH keys are only stored locally on B; proxy execution follows existing whitelist policies
4. **Tunnel Rate Limiting**: Maximum tunnel count limit (default 100), idle tunnels automatically reclaimed (default 300 seconds)
5. **Password Protection**: SSH passwords in configuration files are automatically masked in diff output

---

## 9. Platform UI Changes

### 9.1 Add Host Page

New fields:
- **Jump Host**: Dropdown selection, listing all gateway-type Agents
- **Connection Mode**: Displayed when a jump host is selected
  - Tunnel Mode (C has Agent)
  - Proxy Mode (C has no Agent)
  - Auto Detect

### 9.2 Host List

- Display connection method: Direct / Via Gateway
- Display gateway status: Online / Offline

### 9.3 Host Details

- Display tunnel status (tunnel mode)
- Display SSH connection status (proxy mode)
