# OpsAgent Plugin SDK Development Guide

> This document is intended for plugin developers, explaining how to use the SDK provided by OpsAgent to write custom plugins. OpsAgent is a host-side metric collection and sandbox execution Agent. Its plugin system implements communication between external processes and the main Agent via UDS (Unix Domain Socket) JSON-RPC 2.0 protocol.

---

## Table of Contents

1. [Overview](#1-overview)
2. [Plugin Manifest (plugin.yaml)](#2-plugin-manifest-pluginyaml)
3. [Go SDK Development](#3-go-sdk-development)
4. [Rust SDK Development](#4-rust-sdk-development)
5. [Plugin Deployment](#5-plugin-deployment)
6. [Protocol Reference](#6-protocol-reference)
7. [Debugging and Testing](#7-debugging-and-testing)

---

## 1. Overview

### 1.1 Plugin Architecture

OpsAgent's plugin system uses an **out-of-process** architecture. The PluginGateway component is responsible for plugin discovery, startup, and lifecycle management:

```
┌─────────────────────────────────────────────────────┐
│                  OpsAgent Main Process               │
│                                                       │
│   ┌───────────────────────────────────────────────┐  │
│   │              PluginGateway                     │  │
│   │                                               │  │
│   │  1. Scan plugin.yaml manifests                │  │
│   │  2. Start plugin processes                    │  │
│   │  3. Send JSON-RPC requests via UDS            │  │
│   │  4. Health check / Auto-restart               │  │
│   └───────────┬───────────────┬───────────────────┘  │
│               │               │                       │
└───────────────┼───────────────┼───────────────────────┘
                │               │
         UDS (Unix Socket)  UDS (Unix Socket)
                │               │
    ┌───────────┴───┐   ┌──────┴──────────┐
    │  Go Plugin    │   │  Rust Plugin    │
    │  Process      │   │  Process        │
    │  (go-echo)    │   │  (rust-audit)   │
    └───────────────┘   └─────────────────┘
```

**Core Features**:

- **Language-agnostic**: Any language capable of sending/receiving JSON over Unix Socket can write plugins
- **Process isolation**: Plugins run as independent processes; crashes do not affect the main Agent
- **Auto-discovery**: PluginGateway automatically discovers plugins by scanning `plugin.yaml` files in the plugin directory
- **Health check**: Periodically sends `ping` requests to detect plugin liveness
- **Auto-restart**: Automatically restarts plugins after abnormal exits, with exponential backoff (max 3 retries)

### 1.2 Built-in Task Types

OpsAgent's Rust runtime includes 6 built-in task types that can be used without writing plugins:

| Task Type | Description |
|----------|------|
| `plugin_log_parse` | Parse log text, count errors/warnings |
| `plugin_text_process` | Text operations: uppercase conversion, lowercase conversion, word count |
| `plugin_fs_scan` | Recursive directory scan, file statistics |
| `plugin_conn_analyze` | Analyze network connections from `/proc/net` |
| `plugin_local_probe` | System health check: disk, memory, OOM, zombie processes |
| `plugin_ebpf_collect` | eBPF syscall counting (requires `ebpf` feature enabled at compile time) |

Custom plugins can define their own task type names. PluginGateway routes requests to the corresponding plugin process based on the `task_types` declared in `plugin.yaml`.

---

## 2. Plugin Manifest (plugin.yaml)

Each plugin must provide a `plugin.yaml` manifest file in the plugin directory to describe the plugin's metadata and runtime configuration.

### 2.1 Full Format

```yaml
# Plugin unique name
name: go-echo

# Semantic version
version: "1.0.0"

# Plugin description
description: "Echo plugin for testing the SDK"

# Author information
author: "opsagent@example.com"

# Runtime type (currently only process is supported)
runtime: process

# Plugin executable path (relative to plugin directory)
binary_path: ./go-echo

# List of task types this plugin supports
task_types:
  - echo

# Resource limits
limits:
  # Maximum memory usage (MB)
  max_memory_mb: 64
  # Single task timeout (seconds)
  timeout_seconds: 10
```

### 2.2 Field Description

| Field | Type | Required | Description |
|------|------|------|------|
| `name` | string | Yes | Plugin unique identifier, recommend lowercase letters and hyphens |
| `version` | string | Yes | Semantic version |
| `description` | string | No | Plugin functionality description |
| `author` | string | No | Author or maintainer contact |
| `runtime` | string | Yes | Runtime type, currently fixed as `process` |
| `binary_path` | string | Yes | Executable path, relative to plugin directory |
| `task_types` | list[string] | Yes | List of task types the plugin handles, at least one |
| `limits.max_memory_mb` | int | No | Memory limit (MB), default determined by Agent config |
| `limits.timeout_seconds` | int | No | Task timeout (seconds), default determined by Agent config |

### 2.3 Examples

**Go Echo Plugin** (`sdk/examples/go-echo/plugin.yaml`):

```yaml
name: go-echo
version: "1.0.0"
description: "Echo plugin for testing the SDK"
author: "opsagent@example.com"
runtime: process
binary_path: ./go-echo
task_types:
  - echo
limits:
  max_memory_mb: 64
  timeout_seconds: 10
```

**Rust Audit Plugin** (`sdk/examples/rust-audit/plugin.yaml`):

```yaml
name: rust-audit
version: "1.0.0"
description: "System audit plugin for testing the Rust SDK"
author: "opsagent@example.com"
runtime: process
binary_path: ./target/release/rust-audit
task_types:
  - audit
limits:
  max_memory_mb: 128
  timeout_seconds: 30
```

---

## 3. Go SDK Development

### 3.1 Installation

```bash
go get github.com/cy77cc/opsagent/sdk/plugin
```

### 3.2 Handler Interface

The core of the Go SDK is the `Handler` interface (defined in `sdk/plugin/handler.go`). Plugin developers need to implement all methods of this interface:

```go
type Handler interface {
    // Init is called once when the plugin starts. cfg may be nil.
    Init(cfg map[string]interface{}) error

    // TaskTypes returns the list of task type strings this handler supports.
    TaskTypes() []string

    // Execute processes a single task request and returns a response.
    Execute(ctx context.Context, req *TaskRequest) (*TaskResponse, error)

    // Shutdown is called when the plugin is gracefully terminated.
    Shutdown(ctx context.Context) error

    // HealthCheck returns nil to indicate the plugin is healthy.
    HealthCheck(ctx context.Context) error
}
```

### 3.3 Request and Response Structures

**TaskRequest** (defined in `sdk/plugin/protocol.go`):

```go
type TaskRequest struct {
    TaskID   string                 `json:"task_id"`    // Task unique ID
    TaskType string                 `json:"task_type"`  // Task type
    Params   map[string]interface{} `json:"params"`     // Task parameters
    Deadline int64                  `json:"deadline_ms"` // Deadline (Unix millisecond timestamp)
}
```

**TaskResponse**:

```go
type TaskResponse struct {
    TaskID string      `json:"task_id"`          // Task unique ID (must match request)
    Status string      `json:"status"`           // "ok" or "error"
    Data   interface{} `json:"data,omitempty"`   // Return data on success
    Error  string      `json:"error,omitempty"`  // Error message on failure
}
```

### 3.4 Starting the Service

The Go SDK provides two startup functions:

```go
// Start with default options
plugin.Serve(handler)

// Start with custom options
plugin.ServeWithOptions(handler,
    plugin.WithLogger(logger),              // Custom slog.Logger
    plugin.WithGracefulTimeout(30*time.Second), // Custom graceful shutdown timeout
)
```

The `Serve` function reads the Unix Socket path from the `OPSAGENT_PLUGIN_SOCKET` environment variable, initializes the handler, then listens for JSON-RPC requests until it receives a SIGTERM or SIGINT signal.

### 3.5 Complete Example: Echo Plugin

Below is a complete Go echo plugin implementation (source code at `sdk/examples/go-echo/main.go`):

```go
package main

import (
    "context"
    "log"

    "github.com/cy77cc/opsagent/sdk/plugin"
)

type EchoHandler struct{}

func (h *EchoHandler) Init(cfg map[string]interface{}) error {
    log.Println("echo plugin initialized")
    return nil
}

func (h *EchoHandler) TaskTypes() []string {
    return []string{"echo"}
}

func (h *EchoHandler) Execute(_ context.Context, req *plugin.TaskRequest) (*plugin.TaskResponse, error) {
    log.Printf("executing task %s with params: %v", req.TaskID, req.Params)
    return &plugin.TaskResponse{
        TaskID: req.TaskID,
        Status: "ok",
        Data: map[string]interface{}{
            "echo": req.Params,
            "task": req.TaskType,
        },
    }, nil
}

func (h *EchoHandler) Shutdown(_ context.Context) error {
    log.Println("echo plugin shutting down")
    return nil
}

func (h *EchoHandler) HealthCheck(_ context.Context) error {
    return nil
}

func main() {
    if err := plugin.Serve(&EchoHandler{}); err != nil {
        log.Fatalf("serve: %v", err)
    }
}
```

**Build and Deploy**:

```bash
# Build
cd sdk/examples/go-echo
go build -o go-echo .

# Deploy to plugin directory
mkdir -p /etc/opsagent/plugins/go-echo
cp go-echo plugin.yaml /etc/opsagent/plugins/go-echo/
```

### 3.6 Custom Options Example

```go
func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
        Level: slog.LevelDebug,
    }))

    if err := plugin.ServeWithOptions(&MyHandler{},
        plugin.WithLogger(logger),
        plugin.WithGracefulTimeout(30*time.Second),
    ); err != nil {
        log.Fatalf("serve: %v", err)
    }
}
```

---

## 4. Rust SDK Development

### 4.1 Installation

Add dependency in `Cargo.toml`:

```toml
[dependencies]
opsagent-plugin = { path = "../../opsagent-plugin" }
# Or from crates.io (after publication)
# opsagent-plugin = "1.0"

async-trait = "0.1"
serde_json = "1"
tokio = { version = "1", features = ["full"] }
tracing = "0.1"
tracing-subscriber = "0.3"
```

### 4.2 Plugin Trait

The core of the Rust SDK is the `Plugin` trait (defined in `sdk/opsagent-plugin/src/lib.rs`). Plugin developers need to implement this trait:

```rust
#[async_trait]
pub trait Plugin: Send + Sync {
    /// Returns the list of task type strings this plugin supports.
    fn task_types(&self) -> Vec<String>;

    /// Called once when the plugin starts. cfg may be Value::Null.
    async fn init(&self, cfg: Value) -> Result<()>;

    /// Processes a single task request and returns a response.
    async fn execute(&self, req: &TaskRequest) -> Result<TaskResponse>;

    /// Called when the plugin is gracefully terminated.
    async fn shutdown(&self) -> Result<()>;

    /// Returns Ok(()) to indicate the plugin is healthy.
    async fn health_check(&self) -> Result<()>;
}
```

### 4.3 Request and Response Structures

**TaskRequest** (defined in `sdk/opsagent-plugin/src/protocol.rs`):

```rust
pub struct TaskRequest {
    pub task_id: String,      // Task unique ID
    pub task_type: String,    // Task type
    pub params: Value,        // Task parameters (serde_json::Value)
    pub deadline_ms: i64,     // Deadline (Unix millisecond timestamp)
}
```

**TaskResponse**:

```rust
pub struct TaskResponse {
    pub task_id: String,              // Task unique ID (must match request)
    pub status: String,               // "ok" or "error"
    pub data: Option<Value>,          // Return data on success
    pub error: Option<String>,        // Error message on failure
}
```

### 4.4 Error Types

The SDK defines a `PluginError` enum (defined in `sdk/opsagent-plugin/src/error.rs`) for unified error handling:

```rust
#[derive(Error, Debug)]
pub enum PluginError {
    #[error("configuration error: {0}")]
    Config(String),       // Configuration error, maps to JSON-RPC error code -32602

    #[error("execution error: {0}")]
    Execution(String),    // Execution error, maps to JSON-RPC error code -32000

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),  // IO error, maps to JSON-RPC error code -32603

    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),  // JSON error, maps to JSON-RPC error code -32700
}

pub type Result<T> = std::result::Result<T, PluginError>;
```

### 4.5 Starting the Service

The Rust SDK provides two startup functions:

```rust
// Start with default options (graceful shutdown timeout 10 seconds)
opsagent_plugin::serve(plugin).await

// Start with custom options
opsagent_plugin::serve_with_options(plugin, ServeOptions {
    graceful_timeout: Duration::from_secs(30),
}).await
```

The `serve` function reads the Unix Socket path from the `OPSAGENT_PLUGIN_SOCKET` environment variable, initializes the plugin, then listens for JSON-RPC requests until it receives a ctrl-c signal.

### 4.6 Complete Example: Audit Plugin

Below is a complete Rust system audit plugin implementation (source code at `sdk/examples/rust-audit/src/main.rs`):

```rust
use async_trait::async_trait;
use opsagent_plugin::error::Result;
use opsagent_plugin::protocol::{TaskRequest, TaskResponse};
use opsagent_plugin::Plugin;
use serde_json::{json, Value};

struct AuditPlugin;

#[async_trait]
impl Plugin for AuditPlugin {
    fn task_types(&self) -> Vec<String> {
        vec!["audit".into()]
    }

    async fn init(&self, _config: Value) -> Result<()> {
        tracing_subscriber::fmt::init();
        tracing::info!("audit plugin initialized");
        Ok(())
    }

    async fn execute(&self, request: &TaskRequest) -> Result<TaskResponse> {
        tracing::info!(task_id = %request.task_id, "executing audit");

        let disk_usage = get_disk_usage();
        let memory_info = get_memory_info();

        Ok(TaskResponse {
            task_id: request.task_id.clone(),
            status: "ok".into(),
            data: Some(json!({
                "disk": disk_usage,
                "memory": memory_info,
            })),
            error: None,
        })
    }

    async fn shutdown(&self) -> Result<()> {
        tracing::info!("audit plugin shutting down");
        Ok(())
    }

    async fn health_check(&self) -> Result<()> {
        Ok(())
    }
}

fn get_disk_usage() -> Value {
    json!({"status": "ok", "note": "disk check placeholder"})
}

fn get_memory_info() -> Value {
    match std::fs::read_to_string("/proc/meminfo") {
        Ok(content) => {
            let total = parse_meminfo_field(&content, "MemTotal");
            let available = parse_meminfo_field(&content, "MemAvailable");
            json!({
                "total_kb": total,
                "available_kb": available,
            })
        }
        Err(e) => json!({"error": e.to_string()}),
    }
}

fn parse_meminfo_field(content: &str, field: &str) -> Option<u64> {
    for line in content.lines() {
        if line.starts_with(field) {
            return line
                .split_whitespace()
                .nth(1)
                .and_then(|s| s.parse().ok());
        }
    }
    None
}

#[tokio::main]
async fn main() -> Result<()> {
    opsagent_plugin::serve(AuditPlugin).await
}
```

**Build and Deploy**:

```bash
# Build
cd sdk/examples/rust-audit
cargo build --release

# Deploy to plugin directory
mkdir -p /etc/opsagent/plugins/rust-audit
cp target/release/rust-audit plugin.yaml /etc/opsagent/plugins/rust-audit/
```

---

## 5. Plugin Deployment

### 5.1 Directory Structure

Plugins are deployed to the directory specified by `plugin_gateway.plugins_dir` (default `/etc/opsagent/plugins/`). Each plugin occupies a subdirectory containing the manifest file and executable:

```
/etc/opsagent/plugins/
├── go-echo/
│   ├── plugin.yaml
│   └── go-echo
└── rust-audit/
    ├── plugin.yaml
    └── rust-audit
```

### 5.2 Deployment Steps

```bash
# 1. Create plugin directory
sudo mkdir -p /etc/opsagent/plugins/my-plugin

# 2. Copy manifest file and executable
sudo cp plugin.yaml my-plugin /etc/opsagent/plugins/my-plugin/

# 3. Set executable permissions
sudo chmod +x /etc/opsagent/plugins/my-plugin/my-plugin

# 4. Restart OpsAgent (or wait for fsnotify to auto-detect file changes)
sudo systemctl restart opsagent
```

### 5.3 Lifecycle Management

PluginGateway manages plugin lifecycle as follows:

| Phase | Behavior |
|------|------|
| **Discovery** | Scans plugin directory at startup, parses all `plugin.yaml` files |
| **Startup** | Creates Unix Socket for each plugin, sets `OPSAGENT_PLUGIN_SOCKET` environment variable, starts plugin process |
| **Health Check** | Periodically sends `ping` requests to each plugin to detect liveness |
| **Auto-restart** | Automatically restarts plugins after abnormal exits, uses exponential backoff (max 3 retries) |
| **File Watch** | Monitors plugin directory for file changes via fsnotify, automatically discovers new plugins |
| **Graceful Shutdown** | Sends SIGTERM to plugin process when Agent stops, waits for graceful shutdown |

### 5.4 Runtime Constraints

| Constraint | Description |
|------|------|
| Socket Path | Passed to plugin process via `OPSAGENT_PLUGIN_SOCKET` environment variable |
| Socket Permission | 0600 (Owner read/write only) |
| Memory Limit | Determined by `limits.max_memory_mb` in `plugin.yaml` or global config |
| Task Timeout | Determined by `limits.timeout_seconds` in `plugin.yaml` or global config |

### 5.5 Writing Plugins in Other Languages

Since the protocol is standard UDS JSON-RPC 2.0, any language supporting Unix Socket and JSON can write plugins. Core steps:

1. Read Socket path from `OPSAGENT_PLUGIN_SOCKET` environment variable
2. Connect to Unix Socket
3. Read newline-delimited JSON-RPC requests
4. Handle `ping` method (return `"pong"`) and `execute_task` method
5. Write newline-delimited JSON-RPC responses
6. Gracefully shutdown on SIGTERM

**Python Minimal Example**:

```python
import json
import os
import signal
import socket
import sys

socket_path = os.environ["OPSAGENT_PLUGIN_SOCKET"]

server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(socket_path)
server.listen(1)
os.chmod(socket_path, 0o600)

running = True
def handle_signal(sig, frame):
    global running
    running = False

signal.signal(signal.SIGTERM, handle_signal)
signal.signal(signal.SIGINT, handle_signal)

while running:
    try:
        server.settimeout(1.0)
        conn, _ = server.accept()
    except socket.timeout:
        continue

    data = conn.recv(65536).strip()
    if not data:
        conn.close()
        continue

    req = json.loads(data)
    method = req.get("method", "")
    req_id = req.get("id", 0)

    if method == "ping":
        resp = {"id": req_id, "result": "pong"}
    elif method == "execute_task":
        params = req.get("params", {})
        resp = {
            "id": req_id,
            "result": {
                "task_id": params.get("task_id", ""),
                "status": "ok",
                "data": {"echo": params},
            },
        }
    else:
        resp = {
            "id": req_id,
            "error": {"code": -32601, "message": f"method not found: {method}"},
        }

    conn.sendall(json.dumps(resp).encode() + b"\n")
    conn.close()

server.close()
os.unlink(socket_path)
```

---

## 6. Protocol Reference

> For the complete protocol specification, see [plugin-contract.md](./plugin-contract.md).

### 6.1 Transport Layer

- **Protocol**: UDS (Unix Domain Socket) JSON-RPC 2.0
- **Message Format**: Newline-delimited JSON (each request/response on one line, ending with `\n`)
- **Connection Model**: Each connection handles one request-response pair, connection closes after processing

### 6.2 Methods

| Method | Description | Parameters |
|------|------|------|
| `ping` | Health check | Empty object `{}` |
| `execute_task` | Execute task | Object containing `task_id`, `task_type`, `params`, `deadline_ms` |

### 6.3 Request Examples

**Health Check**:

```json
{"jsonrpc":"2.0","method":"ping","id":"health-1","params":{}}
```

**Execute Task**:

```json
{
    "jsonrpc": "2.0",
    "method": "execute_task",
    "id": "task-001",
    "params": {
        "task_id": "task-001",
        "task_type": "echo",
        "params": {"message": "hello"},
        "deadline_ms": 1714300000000
    }
}
```

### 6.4 Response Examples

**Success**:

```json
{"id":"task-001","result":{"task_id":"task-001","status":"ok","data":{"echo":{"message":"hello"}}}}
```

**Error**:

```json
{"id":"task-001","error":{"code":-32602,"message":"Configuration error: root_path is required"}}
```

### 6.5 Error Codes

| Error Code | Meaning | Corresponding Scenario |
|--------|------|----------|
| -32700 | Parse error | Invalid JSON |
| -32600 | Invalid request | Missing required fields |
| -32601 | Method not found | Unknown method name |
| -32602 | Invalid params | Plugin configuration error (`PluginError::Config`) |
| -32603 | Internal error | IO error or serialization error (`PluginError::Io`) |
| -32000 | Server error | Task execution failure (`PluginError::Execution`) |

### 6.6 Large Output Chunking Protocol

When task output is large, the `chunks` field in the response is used for chunked transfer:

```json
{
    "chunks": [
        {"seq": 1, "eof": false, "data_b64": "base64..."},
        {"seq": 2, "eof": false, "data_b64": "base64..."},
        {"seq": 3, "eof": true, "data_b64": "base64..."}
    ]
}
```

| Field | Description |
|------|------|
| `seq` | Sequence number starting from 1 |
| `eof` | `true` on the last chunk |
| `data_b64` | Base64-encoded chunk data |

Client reassembles the complete output by concatenating all `data_b64` in `seq` order.

### 6.7 Chunk Structure in Go SDK

```go
type Chunk struct {
    Seq    int    `json:"seq"`
    EOF    bool   `json:"eof"`
    DataB64 string `json:"data_b64"`
}

type TaskStats struct {
    DurationMS   int64 `json:"duration_ms"`
    CPUMS        int64 `json:"cpu_ms"`
    MemPeakBytes int64 `json:"mem_peak_bytes"`
}
```

---

## 7. Debugging and Testing

### 7.1 Local Testing

Use the `socat` tool to directly connect to the plugin's Unix Socket for manual testing:

```bash
# Connect to plugin socket
socat - UNIX-CONNECT:/tmp/opsagent/plugin.sock

# Send ping request
echo '{"jsonrpc":"2.0","method":"ping","id":"1","params":{}}' | socat - UNIX-CONNECT:/path/to/socket

# Send execute_task request
echo '{"jsonrpc":"2.0","method":"execute_task","id":"2","params":{"task_id":"test-001","task_type":"echo","params":{"msg":"hello"},"deadline_ms":0}}' | socat - UNIX-CONNECT:/path/to/socket
```

### 7.2 Viewing Logs

```bash
# Real-time track OpsAgent logs
sudo journalctl -u opsagent -f

# View today's logs
sudo journalctl -u opsagent --since today

# Filter plugin-related logs
sudo journalctl -u opsagent | grep -i plugin
```

### 7.3 Common Troubleshooting

| Problem | Possible Cause | Troubleshooting Method |
|------|----------|----------|
| `OPSAGENT_PLUGIN_SOCKET environment variable is not set` | Plugin was not started by PluginGateway | Ensure plugin is started through PluginGateway, not manually |
| `permission denied` | Insufficient Socket file permissions | Check if Socket file permissions are 0600, and if the plugin process has access |
| `plugin binary not found` | Incorrect `binary_path` configuration | Check if `binary_path` in `plugin.yaml` is correct relative to the plugin directory |
| Plugin repeatedly restarts | Plugin crashes on startup | View the plugin process stderr output, check initialization logic |
| `method not found` | Task type mismatch | Check if `task_types` in `plugin.yaml` matches the requested `task_type` |
| Task timeout | Execution time exceeds limit | Check `limits.timeout_seconds` configuration, optimize plugin execution logic |

### 7.4 Development Debugging Tips

**1. Manually start plugin locally for debugging**:

```bash
# Set socket path
export OPSAGENT_PLUGIN_SOCKET=/tmp/test-plugin.sock

# Start plugin (foreground, can see log output directly)
./my-plugin
```

In another terminal, use `socat` to send requests for testing.

**2. Enable verbose logging**:

- **Go Plugin**: Use `WithLogger` option to pass a `slog.LevelDebug` level logger
- **Rust Plugin**: Use `tracing_subscriber`'s `EnvFilter` to set `RUST_LOG=debug`

**3. Check if plugin is correctly discovered by PluginGateway**:

```bash
# View the plugin list loaded by PluginGateway
curl http://127.0.0.1:18080/api/v1/health | jq '.plugins'
```

---

## Appendix: Quick Reference

### Go SDK Quick Reference

```go
import "github.com/cy77cc/opsagent/sdk/plugin"

// Implement Handler interface
type MyHandler struct{}
func (h *MyHandler) Init(cfg map[string]interface{}) error { ... }
func (h *MyHandler) TaskTypes() []string { return []string{"my-task"} }
func (h *MyHandler) Execute(ctx context.Context, req *plugin.TaskRequest) (*plugin.TaskResponse, error) { ... }
func (h *MyHandler) Shutdown(ctx context.Context) error { ... }
func (h *MyHandler) HealthCheck(ctx context.Context) error { ... }

// Start
func main() {
    if err := plugin.Serve(&MyHandler{}); err != nil {
        log.Fatalf("serve: %v", err)
    }
}
```

### Rust SDK Quick Reference

```rust
use async_trait::async_trait;
use opsagent_plugin::{Plugin, protocol::{TaskRequest, TaskResponse}, error::Result};
use serde_json::Value;

struct MyPlugin;

#[async_trait]
impl Plugin for MyPlugin {
    fn task_types(&self) -> Vec<String> { vec!["my-task".into()] }
    async fn init(&self, _cfg: Value) -> Result<()> { Ok(()) }
    async fn execute(&self, req: &TaskRequest) -> Result<TaskResponse> { ... }
    async fn shutdown(&self) -> Result<()> { Ok(()) }
    async fn health_check(&self) -> Result<()> { Ok(()) }
}

#[tokio::main]
async fn main() -> Result<()> {
    opsagent_plugin::serve(MyPlugin).await
}
```

### plugin.yaml Quick Reference

```yaml
name: my-plugin
version: "1.0.0"
description: "My custom plugin"
author: "me@example.com"
runtime: process
binary_path: ./my-plugin
task_types:
  - my-task
limits:
  max_memory_mb: 128
  timeout_seconds: 30
```
