# OpsAgent Platform Integration Guide

> This document is intended for platform-side developers, explaining how to deploy OpsAgent Agent and how to write gRPC services on the platform side to receive metrics and dispatch commands.

---

## Table of Contents

1. [Architecture Overview](#1-architecture-overview)
2. [Agent Installation and Deployment](#2-agent-installation-and-deployment)
3. [gRPC Proto Definition](#3-grpc-proto-definition)
4. [Platform-Side Service Implementation](#4-platform-side-service-implementation)
5. [Message Interaction Flow](#5-message-interaction-flow)
6. [Complete Platform-Side Example (Go)](#6-complete-platform-side-example-go)
7. [Configuration Reference](#7-configuration-reference)
8. [Troubleshooting](#8-troubleshooting)
9. [System Health Check](#9-system-health-check)
10. [Health Check Checker Reference](#10-health-check-checker-reference)

---

## 1. Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│                   Platform (Your Service)             │
│                                                       │
│   ┌───────────────────────────────────────────────┐  │
│   │         gRPC Server (AgentService)            │  │
│   │                                               │  │
│   │  ┌─────────────┐    ┌─────────────────────┐  │  │
│   │  │  Receive     │    │  Dispatch           │  │  │
│   │  │  Metrics     │    │  ExecuteCommand     │  │  │
│   │  │  → Store/    │    │  → Wait for         │  │  │
│   │  │    Alert     │    │    ExecResult       │  │  │
│   │  └─────────────┘    └─────────────────────┘  │  │
│   └───────────────────────┬───────────────────────┘  │
└───────────────────────────┼───────────────────────────┘
                            │ Bidirectional Stream
                            │
┌───────────────────────────┼───────────────────────────┐
│                   OpsAgent Agent                     │
│                           │                             │
│   ┌───────────────────────┴───────────────────────┐   │
│   │              gRPC Client                       │   │
│   │  Connect → Register → Heartbeat → Send Metrics│   │
│   │  → Send Results                               │   │
│   └───────────────────────┬───────────────────────┘   │
│                           │                             │
│   ┌───────────┐  ┌───────┴───────┐  ┌─────────────┐  ┌─────────────┐  │
│   │ Collector  │  │   Sandbox     │  │  Executor   │  │  Checker    │  │
│   │ Pipeline   │  │   Executor    │  │  (local)    │  │  Registry   │  │
│   │ CPU/Mem/   │  │   nsjail      │  │  Direct     │  │  Health     │  │
│   │ Disk/Net   │  │   Isolation   │  │  Execution  │  │  Check      │  │
│   │            │  │   Cmd/Script  │  │             │  │  20 Checkers│  │
│   └───────────┘  └───────────────┘  └─────────────┘  └─────────────┘  │
└───────────────────────────────────────────────────────┘
```

**Core Communication Method**: Agent actively connects to the platform's gRPC bidirectional stream, and the platform dispatches commands through the same stream.

---

## 2. Agent Installation and Deployment

### 2.1 System Requirements

| Item | Requirement |
|------|------|
| OS | Linux (amd64/arm64) |
| Go | 1.21+ (compile time only) |
| nsjail | Optional, required for sandbox functionality |
| cgroup v2 | Optional, required for resource limits |

### 2.2 Option 1: Package Deployment (Recommended)

The packaging script cross-compiles installation packages for both x86_64 and arm64 architectures, containing binaries, configuration files, systemd service files, and installation scripts.

**Packaging** (execute on development machine):

```bash
# Package both architectures
make package

# Package a single architecture only
make package-amd64
make package-arm64

# Specify version number
VERSION=1.0.0 make package
```

Artifacts are in the `dist/` directory:

```
dist/
├── opsagent-dev-linux-amd64.tar.gz
├── opsagent-dev-linux-arm64.tar.gz
├── amd64/
│   └── opsagent          # x86_64 binary
└── arm64/
    └── opsagent-arm64    # arm64 binary
```

**Installation** (execute on target machine):

```bash
# Extract
tar xzf opsagent-<version>-linux-amd64.tar.gz
cd opsagent-<version>-linux-amd64

# One-click install (requires root)
sudo ./install.sh
```

The installation script automatically completes:

| Step | Description |
|------|------|
| Install binary | `/usr/local/bin/opsagent` |
| Install config | `/etc/opsagent/config.yaml` (existing files are not overwritten, new config saved as `.new`) |
| Install systemd service | `/etc/systemd/system/opsagent.service` |
| Create log directory | `/var/log/opsagent/` |

After installation, follow the prompts:

```bash
# 1. Edit configuration
sudo vim /etc/opsagent/config.yaml

# 2. Start service
sudo systemctl start opsagent

# 3. Enable auto-start on boot
sudo systemctl enable opsagent

# 4. Check status
sudo systemctl status opsagent

# 5. View logs
sudo journalctl -u opsagent -f
```

### 2.3 Option 2: Build from Source

```bash
git clone <repo-url> opsagent
cd opsagent

# Build for current architecture
make build
# Output: bin/opsagent

# Cross-compile both architectures
make build-all
# Output: bin/opsagent-amd64, bin/opsagent-arm64

# Manual installation
sudo cp bin/opsagent /usr/local/bin/opsagent
sudo mkdir -p /etc/opsagent
sudo cp configs/config.yaml /etc/opsagent/config.yaml
```

### 2.4 Configuration

Configuration file path: `/etc/opsagent/config.yaml`

**Minimal Configuration** (metrics collection + gRPC connection only):

```yaml
agent:
  id: "agent-prod-001"        # Unique identifier, recommend hostname or UUID
  name: "web-server-01"       # Human-readable name
  interval_seconds: 10        # Metric collection interval

server:
  listen_addr: "127.0.0.1:18080"  # Local API listen address

executor:
  timeout_seconds: 10
  max_output_bytes: 65536
  allowed_commands:
    - uptime
    - df
    - free
    - hostname

reporter:
  mode: "stdout"

grpc:
  server_addr: "platform.example.com:443"  # Platform gRPC address
  enroll_token: "your-enrollment-token"     # Enrollment token
  mtls:
    cert_file: "/etc/opsagent/certs/client.crt"
    key_file: "/etc/opsagent/certs/client.key"
    ca_file: "/etc/opsagent/certs/ca.crt"
  heartbeat_interval_seconds: 15
  reconnect_initial_backoff_ms: 1000
  reconnect_max_backoff_ms: 30000

collector:
  inputs:
    - type: cpu
      config:
        totalcpu: true
    - type: memory
      config: {}
    - type: disk
      config: {}
    - type: net
      config: {}
    - type: process
      config:
        top_n: 10
  processors:
    - type: tagger
      config:
        tags:
          env: "production"
          region: "cn-east"
  outputs:
    - type: http
      config:
        url: "https://metrics.example.com/api/v1/push"
        timeout: 5
```

**Enable Sandbox** (requires nsjail):

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
      - df
      - free
    blocked_commands:
      - rm
      - mkfs
      - dd
      - shutdown
    blocked_keywords:
      - "rm -rf /"
    allowed_interpreters:
      - bash
      - python3
    script_max_bytes: 65536
    shell_injection_check: true
```

### 2.5 Systemd Service Management

The installation package includes a systemd service file with the following operations:

```bash
# Start / Stop / Restart
sudo systemctl start opsagent
sudo systemctl stop opsagent
sudo systemctl restart opsagent

# Check status
sudo systemctl status opsagent

# Enable / Disable auto-start
sudo systemctl enable opsagent
sudo systemctl disable opsagent

# View logs
sudo journalctl -u opsagent -f           # Real-time tracking
sudo journalctl -u opsagent --since today # Today's logs
sudo journalctl -u opsagent -n 100        # Last 100 lines
```

Service file features:

| Feature | Description |
|------|------|
| Auto-restart | Automatic restart 5 seconds after crash (`Restart=always`) |
| Network dependency | Waits for network readiness before starting (`After=network-online.target`) |
| Security hardening | `ProtectSystem=strict`, `ProtectHome=true`, `PrivateTmp=true` |
| Logging | Managed via journald, `LOG_LEVEL=info` can be modified in service file |

### 2.6 Uninstallation

The installation package includes an uninstall script that stops the service, deletes binaries and systemd service files, and interactively confirms whether to delete configuration and log directories:

```bash
sudo ./uninstall.sh
```

Uninstallation flow:

| Step | Description |
|------|------|
| Stop service | `systemctl stop opsagent` |
| Disable auto-start | `systemctl disable opsagent` |
| Delete service file | `/etc/systemd/system/opsagent.service` |
| Delete binary | `/usr/local/bin/opsagent` |
| Delete config | `/etc/opsagent/` (interactive confirmation) |
| Delete logs | `/var/log/opsagent/` (interactive confirmation) |
| Delete temp directory | `/tmp/opsagent/` |

### 2.7 Verify Installation

```bash
# Check binary
opsagent --help

# Check sandbox prerequisites (when building from source)
make sandbox-check

# Run smoke test (when building from source)
./scripts/smoke-test.sh

# Check local API
curl http://127.0.0.1:18080/api/v1/health

# Check Prometheus metrics
curl http://127.0.0.1:18080/metrics
```

---

## 3. gRPC Proto Definition

The complete proto definition is in `proto/agent.proto`. Core service:

```protobuf
service AgentService {
  // Agent actively calls to establish bidirectional stream
  rpc Connect(stream AgentMessage) returns (stream PlatformMessage);
}
```

### 3.1 Agent -> Platform (AgentMessage)

Messages sent by Agent to the platform:

```protobuf
message AgentMessage {
  oneof payload {
    AgentRegistration registration = 1;  // Registration on first connection
    Heartbeat heartbeat = 2;             // Periodic heartbeat
    MetricBatch metrics = 3;             // Metric batch
    ExecOutput exec_output = 4;          // Real-time command execution output
    ExecResult exec_result = 5;          // Command execution result
    Ack ack = 6;                         // Acknowledgment message
    HealthCheckResult health_check_result = 7; // Health check result
  }
}
```

| Message Type | Trigger Timing | Key Fields |
|---------|---------|---------|
| `AgentRegistration` | Sent immediately after connection established | `agent_id`, `token`, `agent_info`, `capabilities` |
| `Heartbeat` | Every 15s (configurable) | `agent_id`, `timestamp_ms`, `status`, `agent_info` |
| `MetricBatch` | Each collection cycle | `metrics[]` (name, tags, fields, timestamp_ms, type) |
| `ExecOutput` | Real-time output during command execution | `task_id`, `stream` (stdout/stderr), `data` |
| `ExecResult` | Command execution completed | `task_id`, `exit_code`, `duration_ms`, `timed_out`, `stats` |
| `HealthCheckResult` | Health check result (streaming) | `request_id`, `results[]`, `summary`, `completed` |

### 3.2 Platform -> Agent (PlatformMessage)

Messages sent by the platform to the Agent:

```protobuf
message PlatformMessage {
  oneof payload {
    ExecuteCommand exec_command = 1;  // Execute command
    ExecuteScript exec_script = 2;    // Execute script
    CancelJob cancel_job = 3;         // Cancel task
    ConfigUpdate config_update = 4;   // Configuration update
    Ack ack = 5;                      // Acknowledgment message
    HealthCheckRequest health_check = 6; // Health check request
  }
}
```

| Message Type | Purpose | Key Fields |
|---------|------|---------|
| `ExecuteCommand` | Execute command on Agent | `task_id`, `command`, `args[]`, `env{}`, `timeout_seconds`, `sandbox` |
| `ExecuteScript` | Execute script on Agent | `task_id`, `interpreter`, `script`, `args[]`, `env{}`, `timeout_seconds`, `sandbox` |
| `CancelJob` | Cancel running task | `task_id`, `reason` |
| `ConfigUpdate` | Push configuration update | `config_yaml`, `version` |
| `HealthCheckRequest` | Trigger system health check | `request_id`, `items[]`, `timeout_seconds` |

---

## 4. Platform-Side Service Implementation

### 4.1 Core Logic

The platform side needs to implement the `Connect` method of `AgentService`:

```
1. Agent calls Connect → Platform receives stream
2. Read first message from stream.Recv() → Should be AgentRegistration
3. Validate token, register Agent
4. Start goroutine to loop Recv() processing Agent messages:
   - Heartbeat → Update Agent status
   - MetricBatch → Store metrics
   - ExecOutput → Forward to waiting callers
   - ExecResult → Notify waiting callers
5. Send PlatformMessage via stream.Send()
```

### 4.2 Proto Code Generation

```bash
# Generate Go code from proto
protoc --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/agent.proto
```

Generated code is in the `internal/grpcclient/proto/` directory, including:
- `agent.pb.go` — Message types
- `agent_grpc.pb.go` — gRPC client/server interfaces

---

## 5. Message Interaction Flow

### 5.1 Agent Registration and Heartbeat

```
Agent                                Platform
  │                                     │
  │──── Connect(stream) ──────────────>│
  │                                     │
  │──── AgentRegistration ────────────>│  // agent_id + token + info
  │                                     │  // Validate token, register Agent
  │<──── Ack (success=true) ───────────│
  │                                     │
  │──── Heartbeat ───────────────────>│  // Every 15s
  │                                     │  // Update last_seen
  │──── Heartbeat ───────────────────>│
  │     ...                             │
```

### 5.2 Metric Reporting

```
Agent                                Platform
  │                                     │
  │──── MetricBatch ─────────────────>│  // cpu, memory, disk, net, process
  │                                     │  // Store to time-series database
  │                                     │
  │──── MetricBatch ─────────────────>│  // Next collection cycle
  │     ...                             │
```

**MetricBatch Structure Example**:

```json
{
  "metrics": [
    {
      "name": "cpu",
      "tags": {"cpu": "cpu-total"},
      "fields": [{"key": "usage_percent", "double_value": 45.2}],
      "timestamp_ms": 1714300000000,
      "type": "GAUGE"
    },
    {
      "name": "memory",
      "tags": {},
      "fields": [
        {"key": "total_bytes", "int_value": 17179869184},
        {"key": "used_percent", "double_value": 62.5}
      ],
      "timestamp_ms": 1714300000000,
      "type": "GAUGE"
    }
  ]
}
```

### 5.3 Command Execution Dispatch

```
Platform                             Agent
  │                                     │
  │──── ExecuteCommand ───────────────>│  // task_id + command + args
  │                                     │  // Validate policy (whitelist/blacklist)
  │                                     │  // If sandbox enabled → nsjail isolated execution
  │                                     │  // Otherwise → direct exec
  │                                     │
  │<──── ExecOutput (stdout) ─────────│  // Real-time output (optional)
  │<──── ExecOutput (stdout) ─────────│
  │<──── ExecResult ──────────────────│  // exit_code + duration + stats
  │                                     │
  │──── Ack ─────────────────────────>│  // Acknowledge receipt of result
```

**ExecuteCommand Example**:

```json
{
  "task_id": "task-20260428-001",
  "command": "df",
  "args": ["-h", "/"],
  "env": {"LANG": "C"},
  "timeout_seconds": 10,
  "sandbox": {
    "memory_mb": 128,
    "cpu_quota_pct": 50,
    "max_pids": 32,
    "network_mode": "disabled"
  }
}
```

**ExecResult Example**:

```json
{
  "task_id": "task-20260428-001",
  "exit_code": 0,
  "duration_ms": 120,
  "timed_out": false,
  "truncated": false,
  "killed": false,
  "stats": {
    "peak_memory_bytes": 2048000,
    "cpu_time_user_ms": 10,
    "cpu_time_system_ms": 5,
    "process_count": 1,
    "bytes_written": 1024,
    "bytes_read": 0
  }
}
```

### 5.5 System Health Check

```
Platform                             Agent
  │                                     │
  │──── HealthCheckRequest ───────────>│  // request_id + items[] + timeout
  │                                     │  // Execute checkers one by one
  │                                     │
  │<──── HealthCheckResult (item 1) ──│  // completed=false, single item result
  │<──── HealthCheckResult (item 2) ──│  // completed=false, single item result
  │<──── HealthCheckResult (item 3) ──│  // completed=false, single item result
  │     ...                             │
  │<──── HealthCheckResult (final) ───│  // completed=true, all results + summary
```

**Streaming Behavior**:
- `completed = false`: Intermediate result, sent immediately after each check item completes, `results[]` contains 1 element
- `completed = true`: Final result, `results[]` contains all results, `summary` contains aggregate statistics
- Platform correlates requests and responses via `request_id`

**HealthCheckRequest Example**:

```json
{
  "request_id": "hc-20260507-001",
  "timeout_seconds": 60,
  "items": [
    {
      "id": "check-ip-forward",
      "type": "network_param_check",
      "category": "network",
      "name": "IP Forward",
      "description": "Check if IP forwarding is disabled",
      "params": {"key": "net.ipv4.ip_forward", "expected": "0"},
      "severity": "SEVERITY_HIGH"
    },
    {
      "id": "check-shadow-perm",
      "type": "file_perm_check",
      "category": "filesystem",
      "name": "Shadow File Permission",
      "description": "Check /etc/shadow permissions",
      "params": {"path": "/etc/shadow", "expected_mode": "0640"},
      "severity": "SEVERITY_CRITICAL"
    },
    {
      "id": "check-sshd",
      "type": "service_check",
      "category": "service",
      "name": "SSH Service",
      "description": "Check if sshd is running",
      "params": {"name": "sshd", "expected_status": "active"},
      "severity": "SEVERITY_HIGH"
    }
  ]
}
```

**HealthCheckResult Example** (intermediate result, `completed=false`):

```json
{
  "request_id": "hc-20260507-001",
  "results": [
    {
      "item_id": "check-ip-forward",
      "type": "network_param_check",
      "name": "IP Forward",
      "status": "STATUS_PASS",
      "actual_value": "0",
      "expected_value": "0",
      "message": "net.ipv4.ip_forward is 0 (expected)",
      "remediation": "",
      "severity": "SEVERITY_HIGH",
      "duration_ms": 2
    }
  ],
  "summary": null,
  "completed": false
}
```

**HealthCheckResult Example** (final result, `completed=true`):

```json
{
  "request_id": "hc-20260507-001",
  "results": [
    {
      "item_id": "check-ip-forward",
      "type": "network_param_check",
      "name": "IP Forward",
      "status": "STATUS_PASS",
      "actual_value": "0",
      "expected_value": "0",
      "message": "net.ipv4.ip_forward is 0 (expected)",
      "severity": "SEVERITY_HIGH",
      "duration_ms": 2
    },
    {
      "item_id": "check-shadow-perm",
      "type": "file_perm_check",
      "name": "Shadow File Permission",
      "status": "STATUS_FAIL",
      "actual_value": "0644",
      "expected_value": "0640",
      "message": "/etc/shadow mode is 0644, expected 0640",
      "severity": "SEVERITY_CRITICAL",
      "duration_ms": 1
    },
    {
      "item_id": "check-sshd",
      "type": "service_check",
      "name": "SSH Service",
      "status": "STATUS_PASS",
      "actual_value": "active",
      "expected_value": "active",
      "message": "sshd is active (expected)",
      "severity": "SEVERITY_HIGH",
      "duration_ms": 50
    }
  ],
  "summary": {
    "total": 3,
    "pass": 2,
    "fail": 1,
    "warn": 0,
    "error": 0,
    "skip": 0,
    "total_duration_ms": 53
  },
  "completed": true
}
```

### 5.4 Script Execution Dispatch

```
Platform                             Agent
  │                                     │
  │──── ExecuteScript ────────────────>│  // task_id + interpreter + script
  │                                     │  // Executed via sandbox isolation
  │<──── ExecOutput (stdout) ─────────│  // Real-time streaming output
  │<──── ExecOutput (stdout) ─────────│
  │<──── ExecResult ──────────────────│
```

**ExecuteScript Example**:

```json
{
  "task_id": "task-20260428-002",
  "interpreter": "bash",
  "script": "echo 'Disk usage:' && df -h && echo 'Memory:' && free -h",
  "timeout_seconds": 30,
  "sandbox": {
    "memory_mb": 256,
    "network_mode": "disabled"
  }
}
```

---

## 6. Complete Platform-Side Example (Go)

Below is a complete platform-side gRPC service implementation:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "your-project/proto" // Replace with your proto package path
)

// AgentServer implements the AgentService gRPC service.
type AgentServer struct {
	pb.UnimplementedAgentServiceServer

	mu     sync.RWMutex
	agents map[string]*AgentSession // agent_id → session
}

// AgentSession represents a connected Agent.
type AgentSession struct {
	AgentID  string
	Stream   pb.AgentService_ConnectServer
	Info     *pb.AgentInfo
	LastSeen int64

	// For waiting on command results
	resultCh chan *pb.ExecResult
	outputCh chan *pb.ExecOutput
}

func NewAgentServer() *AgentServer {
	return &AgentServer{
		agents: make(map[string]*AgentSession),
	}
}

// Connect is the core method: handles Agent's bidirectional stream connection.
func (s *AgentServer) Connect(stream pb.AgentService_ConnectServer) error {
	// 1. Read registration message
	regMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to receive registration: %v", err)
	}

	reg := regMsg.GetRegistration()
	if reg == nil {
		return status.Errorf(codes.InvalidArgument, "first message must be registration")
	}

	// 2. Validate token
	if !s.validateToken(reg.GetToken()) {
		// Send failure ack
		stream.Send(&pb.PlatformMessage{
			Payload: &pb.PlatformMessage_Ack{
				Ack: &pb.Ack{
					RefId:   "registration",
					Success: false,
					Error:   "invalid token",
				},
			},
		})
		return status.Errorf(codes.Unauthenticated, "invalid token")
	}

	agentID := reg.GetAgentId()
	log.Printf("[+] Agent connected: %s (host=%s, os=%s)",
		agentID, reg.GetAgentInfo().GetHostname(), reg.GetAgentInfo().GetOs())

	// 3. Register session
	session := &AgentSession{
		AgentID:  agentID,
		Stream:   stream,
		Info:     reg.GetAgentInfo(),
		resultCh: make(chan *pb.ExecResult, 10),
		outputCh: make(chan *pb.ExecOutput, 100),
	}

	s.mu.Lock()
	s.agents[agentID] = session
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.agents, agentID)
		s.mu.Unlock()
		log.Printf("[-] Agent disconnected: %s", agentID)
	}()

	// 4. Send registration success ack
	stream.Send(&pb.PlatformMessage{
		Payload: &pb.PlatformMessage_Ack{
			Ack: &pb.Ack{
				RefId:   "registration",
				Success: true,
			},
		},
	})

	// 5. Message receive loop
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err // Stream closed
		}

		switch p := msg.Payload.(type) {
		case *pb.AgentMessage_Heartbeat:
			hb := p.Heartbeat
			session.LastSeen = hb.GetTimestampMs()
			log.Printf("[HB] %s status=%s", agentID, hb.GetStatus())

		case *pb.AgentMessage_Metrics:
			batch := p.Metrics
			log.Printf("[METRICS] %s: %d metrics", agentID, len(batch.GetMetrics()))
			// TODO: Write to time-series database (InfluxDB/Prometheus/etc.)
			for _, m := range batch.GetMetrics() {
				s.processMetric(agentID, m)
			}

		case *pb.AgentMessage_ExecOutput:
			out := p.ExecOutput
			log.Printf("[OUTPUT] %s [%s]: %s", out.GetTaskId(), out.GetStream(), string(out.GetData()))
			session.outputCh <- out

		case *pb.AgentMessage_ExecResult:
			res := p.ExecResult
			log.Printf("[RESULT] %s: exit_code=%d duration=%dms",
				res.GetTaskId(), res.GetExitCode(), res.GetDurationMs())
			session.resultCh <- res

		case *pb.AgentMessage_Ack:
			ack := p.Ack
			log.Printf("[ACK] %s: success=%v", ack.GetRefId(), ack.GetSuccess())
		}
	}
}

// ExecuteCommand dispatches command execution to a specified Agent.
func (s *AgentServer) ExecuteCommand(ctx context.Context, agentID string, cmd *pb.ExecuteCommand) (*pb.ExecResult, error) {
	s.mu.RLock()
	session, ok := s.agents[agentID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent %s not connected", agentID)
	}

	// Drain previous results
	for {
		select {
		case <-session.resultCh:
		default:
			goto drained
		}
	}
drained:

	// Send command
	msg := &pb.PlatformMessage{
		Payload: &pb.PlatformMessage_ExecCommand{
			ExecCommand: cmd,
		},
	}
	if err := session.Stream.Send(msg); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	// Wait for result (with timeout)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-session.resultCh:
		return result, nil
	}
}

// ExecuteScript dispatches script execution to a specified Agent.
func (s *AgentServer) ExecuteScript(ctx context.Context, agentID string, script *pb.ExecuteScript) (*pb.ExecResult, error) {
	s.mu.RLock()
	session, ok := s.agents[agentID]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent %s not connected", agentID)
	}

	msg := &pb.PlatformMessage{
		Payload: &pb.PlatformMessage_ExecScript{
			ExecScript: script,
		},
	}
	if err := session.Stream.Send(msg); err != nil {
		return nil, fmt.Errorf("send script: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-session.resultCh:
		return result, nil
	}
}

// ListAgents returns all connected Agents.
func (s *AgentServer) ListAgents() []*AgentSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentSession, 0, len(s.agents))
	for _, session := range s.agents {
		result = append(result, session)
	}
	return result
}

func (s *AgentServer) validateToken(token string) bool {
	// TODO: Implement real token validation logic
	return token != ""
}

func (s *AgentServer) processMetric(agentID string, m *pb.Metric) {
	// TODO: Write to your time-series database
	// Examples: InfluxDB, Prometheus Remote Write, VictoriaMetrics, etc.
	log.Printf("  metric: %s %v %v", m.GetName(), m.GetTags(), m.GetFields())
}

func main() {
	lis, err := net.Listen("tcp", ":443")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	// TODO: Configure TLS
	srv := grpc.NewServer()
	agentSrv := NewAgentServer()
	pb.RegisterAgentServiceServer(srv, agentSrv)

	log.Println("Platform gRPC server listening on :443")
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
```

### 6.1 Call Examples

```go
// Dispatch command to Agent
result, err := agentSrv.ExecuteCommand(ctx, "agent-prod-001", &pb.ExecuteCommand{
	TaskId:         "task-001",
	Command:        "df",
	Args:           []string{"-h", "/"},
	TimeoutSeconds: 10,
	Sandbox: &pb.SandboxConfig{
		MemoryMb:    128,
		CpuQuotaPct: 50,
		MaxPids:     32,
		NetworkMode: "disabled",
	},
})
if err != nil {
	log.Printf("execute failed: %v", err)
} else {
	log.Printf("exit_code=%d, duration=%dms", result.GetExitCode(), result.GetDurationMs())
}

// Dispatch script to Agent
result, err := agentSrv.ExecuteScript(ctx, "agent-prod-001", &pb.ExecuteScript{
	TaskId:      "task-002",
	Interpreter: "bash",
	Script:      "echo '=== System Info ===' && uname -a && uptime && free -h",
	TimeoutSeconds: 30,
})
```

---

## 7. Configuration Reference

### 7.1 Complete Agent Configuration Fields

| Field | Type | Default | Description |
|------|------|--------|------|
| `agent.id` | string | (required) | Agent unique identifier |
| `agent.name` | string | (required) | Agent human-readable name |
| `agent.interval_seconds` | int | 10 | Metric collection interval (seconds) |
| `server.listen_addr` | string | 0.0.0.0:18080 | Local API listen address |
| `grpc.server_addr` | string | (required) | Platform gRPC address |
| `grpc.enroll_token` | string | "" | Enrollment token |
| `grpc.mtls.cert_file` | string | "" | Client certificate path |
| `grpc.mtls.key_file` | string | "" | Client private key path |
| `grpc.mtls.ca_file` | string | "" | CA certificate path |
| `grpc.heartbeat_interval_seconds` | int | 15 | Heartbeat interval |
| `grpc.reconnect_initial_backoff_ms` | int | 1000 | Reconnect initial backoff (ms) |
| `grpc.reconnect_max_backoff_ms` | int | 30000 | Reconnect max backoff (ms) |
| `sandbox.enabled` | bool | false | Enable sandbox |
| `sandbox.nsjail_path` | string | /usr/bin/nsjail | nsjail path |
| `sandbox.default_timeout_seconds` | int | 30 | Default execution timeout |
| `sandbox.max_concurrent_tasks` | int | 4 | Max concurrent tasks |
| `collector.inputs[]` | list | - | Collection plugin list |
| `collector.processors[]` | list | - | Processor plugin list |
| `collector.outputs[]` | list | - | Output plugin list |

### 7.2 Available Collection Plugins

| Plugin | type | Optional config |
|------|------|------------|
| CPU | `cpu` | `totalcpu: true`, `percpu: false` |
| Memory | `memory` | None |
| Disk | `disk` | `mount_points: ["/", "/data"]` |
| Network | `net` | None |
| Process | `process` | `top_n: 10` |

### 7.3 Available Processor Plugins

| Plugin | type | config |
|------|------|--------|
| Tagger | `tagger` | `tags: {env: "prod", region: "east"}` |
| Regex | `regex` | `tags: [{key: "host", pattern: "...", replacement: "..."}]` |

### 7.4 Available Aggregator Plugins

| Plugin | type | config |
|------|------|--------|
| Average | `avg` | `fields: ["usage_percent"]` |
| Sum | `sum` | `fields: ["bytes_sent"]` |

### 7.5 Available Output Plugins

| Plugin | type | config |
|------|------|--------|
| HTTP | `http` | `url`, `timeout`, `batch_size`, `retry_count` |
| Prometheus | `prometheus` | `path`, `addr` |
| Prometheus Remote Write | `prometheus_remote_write` | `url`, `timeout` |

---

## 8. Troubleshooting

### 8.1 Agent Cannot Connect to Platform

```bash
# Check network connectivity
nc -zv platform.example.com 443

# Check certificate
openssl x509 -in /etc/opsagent/certs/client.crt -noout -dates

# View Agent logs (enable debug)
LOG_LEVEL=debug ./bin/opsagent run --config /etc/opsagent/config.yaml
```

### 8.2 Sandbox Command Rejected

```bash
# Check nsjail
which nsjail
nsjail --version

# Check cgroup
cat /sys/fs/cgroup/cgroup.controllers

# Check Agent logs for policy errors
journalctl -u opsagent | grep "policy"
```

### 8.3 Metrics Not Reaching Platform

```bash
# Check Agent local Prometheus endpoint
curl http://127.0.0.1:18080/metrics

# Check collector configuration
# Ensure at least one input is correctly configured in collector.inputs

# Check gRPC connection status
curl http://127.0.0.1:18080/api/v1/health
```

### 8.4 Common Error Codes

| Scenario | exit_code | Description |
|------|-----------|------|
| Normal exit | 0 | Command executed successfully |
| Command error | 1-125 | Error code returned by the command itself |
| Killed by timeout | -1 | Execution timed out, `timed_out=true` |
| Policy rejected | N/A | gRPC returns error, no ExecResult generated |
| Sandbox error | N/A | cgroup/nsjail configuration issue |

---

## 9. System Health Check

### 9.1 Feature Overview

System health check allows the platform to dispatch a set of check items to the Agent, which executes them one by one on the host and returns results in a streaming manner. The platform can customize checks for kernel parameters, file permissions, network configuration, service status, container runtime, and more.

**Relationship with Existing Features**:

| Feature | `/healthz` Endpoint | System Health Check |
|------|-----------------|-------------|
| Check target | Agent's own subsystems (gRPC, scheduler, plugins) | Host system configuration |
| Trigger method | HTTP GET | gRPC message |
| Scope | Agent health status | OS/kernel/network/service/container configuration |
| Customizable | No | Yes — Platform defines check items |

### 9.2 Capability Discovery

Agent declares supported checker types in `capabilities` during registration:

```json
{
  "capabilities": [
    "health_check",
    "checker:sysctl_check",
    "checker:kernel_version_check",
    "checker:kernel_module_check",
    "checker:boot_param_check",
    "checker:file_perm_check",
    "checker:file_exist_check",
    "checker:dir_perm_check",
    "checker:mount_option_check",
    "checker:port_check",
    "checker:ssh_config_check",
    "checker:iptables_check",
    "checker:network_param_check",
    "checker:service_check",
    "checker:user_check",
    "checker:cron_check",
    "checker:pam_check",
    "checker:docker_check",
    "checker:containerd_check",
    "checker:cgroup_check",
    "checker:container_runtime_check"
  ]
}
```

The platform should check the Agent's `capabilities` before sending health check requests to confirm the target checkers are available.

### 9.3 Platform-Side Implementation

Add `HealthCheckResult` handling to the existing `Connect` message loop:

```go
case *pb.AgentMessage_HealthCheckResult:
    result := p.HealthCheckResult
    reqID := result.GetRequestId()

    if result.GetCompleted() {
        // Final result: contains all results and summary
        log.Printf("[HC-DONE] %s: total=%d pass=%d fail=%d",
            reqID, result.GetSummary().GetTotal(),
            result.GetSummary().GetPass(), result.GetSummary().GetFail())
        // Notify waiting callers
        session.healthCh <- result
    } else {
        // Intermediate result: single item completed
        for _, r := range result.GetResults() {
            log.Printf("[HC-ITEM] %s: %s status=%s msg=%s",
                reqID, r.GetItemId(), r.GetStatus(), r.GetMessage())
        }
    }
```

### 9.4 Dispatching Health Check Requests

```go
// HealthCheck dispatches a health check to a specified Agent.
func (s *AgentServer) HealthCheck(ctx context.Context, agentID string,
    req *pb.HealthCheckRequest) (*pb.HealthCheckResult, error) {

    s.mu.RLock()
    session, ok := s.agents[agentID]
    s.mu.RUnlock()

    if !ok {
        return nil, fmt.Errorf("agent %s not connected", agentID)
    }

    msg := &pb.PlatformMessage{
        Payload: &pb.PlatformMessage_HealthCheck{
            HealthCheck: req,
        },
    }
    if err := session.Stream.Send(msg); err != nil {
        return nil, fmt.Errorf("send health check: %w", err)
    }

    // Wait for final result (completed=true)
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case result := <-session.healthCh:
        return result, nil
    }
}
```

A channel needs to be added to `AgentSession`:

```go
type AgentSession struct {
    // ... existing fields ...
    healthCh chan *pb.HealthCheckResult // Health check final result
}
```

During initialization:

```go
session := &AgentSession{
    // ... existing fields ...
    healthCh: make(chan *pb.HealthCheckResult, 5),
}
```

### 9.5 Configuration

Agent-side health check configuration:

```yaml
checker:
  enabled: true                    # Enable health check feature
  max_concurrent: 5                # Max concurrent checks (reserved)
  default_timeout_seconds: 30      # Default timeout
  disabled_checkers: []            # List of disabled checker types
```

### 9.6 Error Handling

| Scenario | Behavior |
|------|------|
| Unknown checker type | That item returns `STATUS_ERROR`, does not interrupt overall request |
| Checker execution error | That item returns `STATUS_ERROR`, continues with remaining items |
| Parameter validation failure | That item returns `STATUS_ERROR`, error message in `message` |
| Overall timeout | Cancels remaining check items, returns completed results + summary |
| Checker not registered | Same as unknown type, returns `STATUS_ERROR` |

### 9.7 Security

- **Parameter Validation**: Each checker validates parameter format at entry
- **Path Traversal Protection**: File path checkers use `filepath.Clean()` + prefix whitelist
- **Timeout Control**: Supports both overall request timeout and per-item timeout
- **No Privilege Escalation**: Checkers run with Agent process permissions (typically root)
- **Audit Log**: Each health check request records `request_id`, `item_count`, `duration`

---

## 10. Health Check Checker Reference

### 10.1 Common Checker Structure

Each check item (`CheckItem`) contains:

| Field | Type | Description |
|------|------|------|
| `id` | string | Platform-defined unique identifier, returned as-is in results |
| `type` | string | Checker type, determines which checker to use |
| `category` | string | Category (display only, does not affect routing) |
| `name` | string | Human-readable name |
| `description` | string | Check description |
| `params` | bytes | JSON-encoded checker parameters |
| `severity` | enum | Severity: `SEVERITY_INFO` / `LOW` / `MEDIUM` / `HIGH` / `CRITICAL` |

Each check result (`CheckResult`) contains:

| Field | Type | Description |
|------|------|------|
| `item_id` | string | Corresponds to CheckItem.id |
| `type` | string | Checker type |
| `name` | string | Check item name |
| `status` | enum | `STATUS_PASS` / `FAIL` / `WARN` / `ERROR` / `SKIP` |
| `actual_value` | string | Actual detected value |
| `expected_value` | string | Expected value |
| `message` | string | Human-readable result description |
| `remediation` | string | Remediation advice (provided by some checkers) |
| `severity` | enum | Severity (returned as-is) |
| `duration_ms` | int64 | Check duration (milliseconds) |

### 10.2 Kernel and System Parameters (`kernel`)

#### sysctl_check

Reads kernel parameter values under `/proc/sys/` and compares with expected values.

**params**:

```json
{
  "path": "/proc/sys/net/ipv4/ip_forward",
  "expected": "0"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `path` | string | Yes | Path under `/proc/sys/` |
| `expected` | string | Yes | Expected value |

**Example**: Check if IP forwarding is disabled

```json
{"path": "/proc/sys/net/ipv4/ip_forward", "expected": "0"}
```

#### kernel_version_check

Gets current kernel version (informational check, always returns `STATUS_PASS`).

**params**: `{}` or omitted

#### kernel_module_check

Checks if a kernel module is loaded.

**params**:

```json
{
  "module": "dccp",
  "expected": "not_loaded"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `module` | string | Yes | Kernel module name |
| `expected` | string | Yes | `"loaded"` or `"not_loaded"` |

#### boot_param_check

Checks boot parameters in `/proc/cmdline`.

**params**:

```json
{
  "param": "selinux",
  "expected": "1"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `param` | string | Yes | Boot parameter name |
| `expected` | string | Yes | Expected value (bare flag returns `"1"`) |

### 10.3 Filesystem Security (`filesystem`)

#### file_perm_check

Checks file permissions (uses `os.Lstat`, does not follow symlinks).

**params**:

```json
{
  "path": "/etc/shadow",
  "expected_mode": "0640"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `path` | string | Yes | File path (must be clean path) |
| `expected_mode` | string | Yes | 4-digit octal permission, e.g. `"0644"` |

#### file_exist_check

Checks if a file exists.

**params**:

```json
{
  "path": "/etc/docker/daemon.json",
  "expected": "exists"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `path` | string | Yes | File path (must be clean path) |
| `expected` | string | Yes | `"exists"` or `"not_exists"` |

#### dir_perm_check

Checks directory permissions and sticky bit.

**params**:

```json
{
  "path": "/tmp",
  "expected_mode": "1777",
  "sticky_bit": true
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `path` | string | Yes | Directory path (must be clean path) |
| `expected_mode` | string | Yes | 4-digit octal permission (including special bits) |
| `sticky_bit` | bool | No | Omit to skip sticky bit check |

#### mount_option_check

Checks if a mount point has specified options (parses `/proc/mounts`).

**params**:

```json
{
  "mount_point": "/tmp",
  "expected_option": "noexec"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `mount_point` | string | Yes | Mount point path |
| `expected_option` | string | Yes | Expected mount option to exist |

### 10.4 Network Security Configuration (`network`)

#### port_check

Checks if a TCP port is listening (parses `/proc/net/tcp` and `/proc/net/tcp6`).

**params**:

```json
{
  "port": 22,
  "expected_state": "listening"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `port` | int | Yes | Port number (1-65535) |
| `expected_state` | string | Yes | `"listening"` or `"not_listening"` |

#### ssh_config_check

Checks SSH configuration items (parses `/etc/ssh/sshd_config`).

**params**:

```json
{
  "key": "PermitRootLogin",
  "expected": "no"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `key` | string | Yes | Configuration item name (case-insensitive) |
| `expected` | string | Yes | Expected value |

#### iptables_check

Checks the default policy of an iptables chain (executes `iptables -L <chain> -n`).

**params**:

```json
{
  "chain": "INPUT",
  "expected_policy": "DROP"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `chain` | string | Yes | Chain name: `"INPUT"` / `"OUTPUT"` / `"FORWARD"` |
| `expected_policy` | string | Yes | Expected policy: `"ACCEPT"` / `"DROP"` / `"REJECT"` |

#### network_param_check

Checks network kernel parameters (converts sysctl format to `/proc/sys/` path).

**params**:

```json
{
  "key": "net.ipv4.ip_forward",
  "expected": "0"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `key` | string | Yes | Parameter name in sysctl format, e.g. `"net.ipv4.ip_forward"` |
| `expected` | string | Yes | Expected value |

### 10.5 Service and Account (`service`)

#### service_check

Checks systemd service status (executes `systemctl is-active`).

**params**:

```json
{
  "name": "sshd",
  "expected_status": "active"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `name` | string | Yes | Service name |
| `expected_status` | string | Yes | Expected status: `"active"` / `"inactive"` / `"failed"` etc. |

#### user_check

Checks user account status (parses `/etc/passwd` + `/etc/shadow`).

**params**:

```json
{
  "username": "root",
  "check": "exists"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `username` | string | Yes | Username |
| `check` | string | Yes | `"exists"` (whether exists) or `"locked"` (whether locked) |

#### cron_check

Audits a user's crontab (informational check, always returns `STATUS_PASS`).

**params**:

```json
{
  "user": "root"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `user` | string | Yes | Username |

#### pam_check

Checks if PAM configuration references a specified module (reads files under `/etc/pam.d/`).

**params**:

```json
{
  "module": "pam_wheel.so",
  "file": "su"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `module` | string | Yes | PAM module name |
| `file` | string | Yes | PAM configuration filename (without path, reads `/etc/pam.d/<file>`) |

### 10.6 Container Runtime (`container`)

#### docker_check

Checks Docker daemon configuration (parses `/etc/docker/daemon.json`).

**params**:

```json
{
  "key": "storage-driver",
  "expected": "overlay2"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `key` | string | Yes | JSON configuration key name |
| `expected` | string | Yes | Expected value |

#### containerd_check

Checks containerd configuration (parses `/etc/containerd/config.toml`).

**params**:

```json
{
  "key": "SystemdCgroup",
  "expected": "true"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `key` | string | Yes | TOML configuration key name |
| `expected` | string | Yes | Expected value |

#### cgroup_check

Detects cgroup version (informational check, always returns `STATUS_PASS`).

**params**: `{}` or omitted

#### container_runtime_check

Checks if a container runtime socket exists.

**params**:

```json
{
  "runtime": "docker",
  "expected": "available"
}
```

| Field | Type | Required | Description |
|------|------|------|------|
| `runtime` | string | Yes | Runtime name: `"docker"` / `"containerd"` / `"cri-o"` |
| `expected` | string | Yes | `"available"` or `"not_available"` |

**Runtime and Socket Path Mapping**:

| Runtime | Socket Path |
|--------|------------|
| `docker` | `/var/run/docker.sock` |
| `containerd` | `/var/run/containerd/containerd.sock` |
| `cri-o` | `/var/run/crio/crio.sock` |

### 10.7 Quick Reference Table

| Type | Category | Parameters | Description |
|------|------|------|------|
| `sysctl_check` | kernel | `path`, `expected` | Kernel parameter value |
| `kernel_version_check` | kernel | None | Kernel version (informational) |
| `kernel_module_check` | kernel | `module`, `expected` | Kernel module load status |
| `boot_param_check` | kernel | `param`, `expected` | Boot parameter |
| `file_perm_check` | filesystem | `path`, `expected_mode` | File permission |
| `file_exist_check` | filesystem | `path`, `expected` | File existence |
| `dir_perm_check` | filesystem | `path`, `expected_mode`, `sticky_bit` | Directory permission |
| `mount_option_check` | filesystem | `mount_point`, `expected_option` | Mount option |
| `port_check` | network | `port`, `expected_state` | Port listening status |
| `ssh_config_check` | network | `key`, `expected` | SSH configuration item |
| `iptables_check` | network | `chain`, `expected_policy` | Firewall rule |
| `network_param_check` | network | `key`, `expected` | Network kernel parameter |
| `service_check` | service | `name`, `expected_status` | systemd service status |
| `user_check` | service | `username`, `check` | User account status |
| `cron_check` | service | `user` | crontab audit (informational) |
| `pam_check` | service | `module`, `file` | PAM configuration check |
| `docker_check` | container | `key`, `expected` | Docker daemon configuration |
| `containerd_check` | container | `key`, `expected` | containerd configuration |
| `cgroup_check` | container | None | cgroup version (informational) |
| `container_runtime_check` | container | `runtime`, `expected` | Runtime socket availability |
