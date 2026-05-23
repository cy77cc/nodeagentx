# Gateway Tunnel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable OpsAgent on host B to act as a jump host, transparently tunneling gRPC connections and proxying commands for internal network hosts that the platform cannot reach directly.

**Architecture:** B's agent gets a new Gateway module with two sub-systems: (1) Tunnel mode — B listens for C's agent connections and relays their gRPC traffic to the platform via extended `AgentMessage`/`PlatformMessage` oneof variants; (2) Proxy mode — B registers C with the platform and executes commands on C's behalf via SSH. Both modes are transparent to the platform.

**Tech Stack:** Go, gRPC (protobuf), SSH (golang.org/x/crypto/ssh), existing OpsAgent patterns (zerolog, prometheus, lumberjack)

---

## File Structure

### New Files

| File | Responsibility |
|---|---|
| `internal/gateway/gateway.go` | Gateway module entry, lifecycle (Start/Stop), HealthStatus, TCP listener |
| `internal/gateway/config.go` | GatewayConfig, HostConfig types |
| `internal/gateway/tunnel/tunnel.go` | Single tunnel: TCP ↔ gRPC relay |
| `internal/gateway/tunnel/pool.go` | Tunnel pool: create/remove/find, max limits |
| `internal/gateway/proxy/proxy.go` | Proxy manager: register hosts, route commands |
| `internal/gateway/proxy/ssh.go` | SSH client: connect, execute, collect metrics |
| `internal/gateway/gateway_test.go` | Unit tests for gateway module |
| `internal/gateway/tunnel/tunnel_test.go` | Unit tests for tunnel |
| `internal/gateway/tunnel/pool_test.go` | Unit tests for pool |
| `internal/gateway/proxy/proxy_test.go` | Unit tests for proxy |
| `internal/gateway/proxy/ssh_test.go` | Unit tests for SSH client |

### Modified Files

| File | Changes |
|---|---|
| `proto/agent.proto` | Add TunnelOpen, TunnelData, TunnelClose, ProxyHostCommand messages |
| `internal/grpcclient/proto/agent.pb.go` | Regenerated from proto |
| `internal/grpcclient/proto/agent_grpc.pb.go` | Regenerated from proto |
| `internal/config/config.go` | Add GatewayConfig, GatewayHostConfig, validation, defaults |
| `internal/config/diff.go` | Add gateway to non-reloadable changes |
| `internal/grpcclient/receiver.go` | Add TunnelDataHandler, ProxyCommandHandler |
| `internal/grpcclient/sender.go` | Add NewTunnelOpenMessage, NewTunnelDataMessage, NewTunnelCloseMessage |
| `internal/app/interfaces.go` | Add Gateway interface |
| `internal/app/agent.go` | Wire gateway into lifecycle |
| `internal/app/options.go` | Add WithGateway option |
| `internal/app/metrics.go` | Add gateway Prometheus metrics |
| `internal/server/server.go` | Add Gateway to HealthCheckers |
| `internal/server/handlers.go` | Add gateway to /healthz |
| `configs/config.yaml` | Add gateway config example |

---

## Task 1: Proto Definitions

**Files:**
- Modify: `proto/agent.proto`

- [ ] **Step 1: Add tunnel messages to proto**

Add the following to `proto/agent.proto` after the existing `HealthCheckResult` message:

```proto
// --- Gateway Tunnel Messages ---

message TunnelOpen {
  string tunnel_id = 1;
  string agent_id = 2;       // C's agent ID
  string hostname = 3;
  string ip = 4;
  repeated string capabilities = 5;
}

message TunnelData {
  string tunnel_id = 1;
  bytes payload = 2;         // Serialized AgentMessage or PlatformMessage
}

message TunnelClose {
  string tunnel_id = 1;
  string reason = 2;
}

message ProxyHostRegister {
  string host_id = 1;
  string hostname = 2;
  string ip = 3;
  repeated string capabilities = 4;
}

message ProxyCommandRequest {
  string host_id = 1;
  string command = 2;
  repeated string args = 3;
  int32 timeout_seconds = 4;
}

message ProxyCommandResponse {
  string host_id = 1;
  string command = 2;
  int32 exit_code = 3;
  bytes stdout = 4;
  bytes stderr = 5;
  int64 duration_ms = 6;
  bool timed_out = 7;
}

message ProxyMetricBatch {
  string host_id = 1;
  repeated Metric metrics = 2;
}
```

- [ ] **Step 2: Extend AgentMessage oneof**

In the `AgentMessage` message, add to the `oneof payload`:

```proto
message AgentMessage {
  oneof payload {
    AgentRegistration registration = 1;
    Heartbeat heartbeat = 2;
    MetricBatch metrics = 3;
    ExecOutput exec_output = 4;
    ExecResult exec_result = 5;
    Ack ack = 6;
    HealthCheckResult health_check_result = 7;
    // Gateway tunnel
    TunnelOpen tunnel_open = 8;
    TunnelData tunnel_data = 9;
    TunnelClose tunnel_close = 10;
    ProxyHostRegister proxy_register = 11;
    ProxyCommandResponse proxy_response = 12;
    ProxyMetricBatch proxy_metrics = 13;
  }
}
```

- [ ] **Step 3: Extend PlatformMessage oneof**

In the `PlatformMessage` message, add to the `oneof payload`:

```proto
message PlatformMessage {
  oneof payload {
    ExecuteCommand exec_command = 1;
    ExecuteScript exec_script = 2;
    CancelJob cancel_job = 3;
    ConfigUpdate config_update = 4;
    Ack ack = 5;
    HealthCheckRequest health_check = 6;
    // Gateway tunnel
    TunnelData tunnel_data = 7;
    TunnelClose tunnel_close = 8;
    ProxyCommandRequest proxy_command = 9;
  }
}
```

- [ ] **Step 4: Regenerate Go protobuf code**

Run:
```bash
make proto
```

Expected: Generated files in `internal/grpcclient/proto/` are updated.

- [ ] **Step 5: Verify build compiles**

Run:
```bash
go build ./...
```

Expected: Compiles (new message types are available but unused).

- [ ] **Step 6: Commit**

```bash
git add proto/agent.proto internal/grpcclient/proto/
git commit -m "feat(proto): add gateway tunnel message types"
```

---

## Task 2: Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/diff.go`
- Modify: `configs/config.yaml`

- [ ] **Step 1: Add GatewayConfig types**

Add to `internal/config/config.go` after `CheckerConfig`:

```go
// GatewayConfig controls the gateway/tunnel subsystem.
type GatewayConfig struct {
	Enabled              bool              `mapstructure:"enabled"`
	ListenAddr           string            `mapstructure:"listen_addr"`
	MaxTunnels           int               `mapstructure:"max_tunnels"`
	TunnelTimeoutSeconds int               `mapstructure:"tunnel_timeout_seconds"`
	IdleTimeoutSeconds   int               `mapstructure:"idle_timeout_seconds"`
	Hosts                []GatewayHostConfig `mapstructure:"hosts"`
}

// GatewayHostConfig defines an internal host behind this gateway.
type GatewayHostConfig struct {
	ID       string            `mapstructure:"id"`
	Addr     string            `mapstructure:"addr"`
	Mode     string            `mapstructure:"mode"` // "tunnel", "proxy", "auto"
	SSH      GatewaySSHConfig  `mapstructure:"ssh"`
}

// GatewaySSHConfig holds SSH credentials for proxy mode.
type GatewaySSHConfig struct {
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	KeyFile  string `mapstructure:"key_file"`
	Port     int    `mapstructure:"port"`
}
```

- [ ] **Step 2: Add Gateway to root Config struct**

In the `Config` struct, add:

```go
Gateway    GatewayConfig    `mapstructure:"gateway"`
```

- [ ] **Step 3: Add defaults in Load()**

Add after the existing `v.SetDefault` calls:

```go
v.SetDefault("gateway.enabled", false)
v.SetDefault("gateway.listen_addr", ":18081")
v.SetDefault("gateway.max_tunnels", 100)
v.SetDefault("gateway.tunnel_timeout_seconds", 30)
v.SetDefault("gateway.idle_timeout_seconds", 300)
```

- [ ] **Step 4: Add validation in Validate()**

Add after the checker validation block:

```go
// Gateway validation (only when enabled).
if c.Gateway.Enabled {
	if strings.TrimSpace(c.Gateway.ListenAddr) == "" {
		return fmt.Errorf("gateway.listen_addr is required when gateway.enabled=true")
	}
	if c.Gateway.MaxTunnels <= 0 {
		return fmt.Errorf("gateway.max_tunnels must be > 0 when gateway.enabled=true")
	}
	if c.Gateway.TunnelTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.tunnel_timeout_seconds must be > 0 when gateway.enabled=true")
	}
	if c.Gateway.IdleTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.idle_timeout_seconds must be > 0 when gateway.enabled=true")
	}
	for i, h := range c.Gateway.Hosts {
		if h.ID == "" {
			return fmt.Errorf("gateway.hosts[%d].id is required", i)
		}
		if h.Addr == "" {
			return fmt.Errorf("gateway.hosts[%d].addr is required", i)
		}
		switch h.Mode {
		case "tunnel", "proxy", "auto":
		default:
			return fmt.Errorf("gateway.hosts[%d].mode must be one of: tunnel, proxy, auto", i)
		}
		if h.Mode == "proxy" || h.Mode == "auto" {
			if h.SSH.User == "" {
				return fmt.Errorf("gateway.hosts[%d].ssh.user is required when mode=%s", i, h.Mode)
			}
			if h.SSH.Port <= 0 {
				return fmt.Errorf("gateway.hosts[%d].ssh.port must be > 0 when mode=%s", i, h.Mode)
			}
		}
	}
}
```

- [ ] **Step 5: Add gateway to non-reloadable diff**

In `internal/config/diff.go`, add after the plugin diff block:

```go
if !reflect.DeepEqual(old.Gateway, new.Gateway) {
	nonReloadable = append(nonReloadable, NonReloadableChange{
		Field:  "gateway.*",
		OldVal: old.Gateway,
		NewVal: new.Gateway,
	})
}
```

- [ ] **Step 6: Update config.yaml example**

Add to `configs/config.yaml`:

```yaml
# Gateway (jump host mode)
gateway:
  enabled: false
  listen_addr: ":18081"          # C's Agent 连入端口（隧道模式）
  max_tunnels: 100               # 最大隧道数
  tunnel_timeout_seconds: 30     # 隧道建立超时
  idle_timeout_seconds: 300      # 空闲隧道回收
  hosts: []                      # 内网主机列表（代理模式）
  #  - id: "vm-web-01"
  #    addr: "192.168.122.100"
  #    mode: "auto"
  #    ssh:
  #      user: "root"
  #      key_file: "/etc/opsagent/keys/id_rsa"
  #      port: 22
```

- [ ] **Step 7: Run tests**

Run:
```bash
go test ./internal/config/... -v -race
```

Expected: All existing config tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/config/ configs/
git commit -m "feat(config): add gateway configuration with validation"
```

---

## Task 3: Gateway Core Types

**Files:**
- Create: `internal/gateway/config.go`
- Create: `internal/gateway/gateway.go`

- [ ] **Step 1: Create gateway config types**

Create `internal/gateway/config.go`:

```go
package gateway

import "time"

// Config holds gateway module configuration.
type Config struct {
	ListenAddr    string
	MaxTunnels    int
	TunnelTimeout time.Duration
	IdleTimeout   time.Duration
	Hosts         []HostConfig
}

// HostConfig defines an internal host.
type HostConfig struct {
	ID   string
	Addr string
	Mode string // "tunnel", "proxy", "auto"
	SSH  SSHConfig
}

// SSHConfig holds SSH connection credentials.
type SSHConfig struct {
	User     string
	Password string
	KeyFile  string
	Port     int
}
```

- [ ] **Step 2: Create gateway module with interfaces**

Create `internal/gateway/gateway.go`:

```go
package gateway

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"

	"github.com/cy77cc/opsagent/internal/health"
)

// TunnelSender sends tunnel messages to the platform via gRPC.
type TunnelSender interface {
	SendTunnelOpen(tunnelID, agentID, hostname, ip string, capabilities []string) error
	SendTunnelData(tunnelID string, payload []byte) error
	SendTunnelClose(tunnelID, reason string) error
}

// ProxySender sends proxy responses to the platform via gRPC.
type ProxySender interface {
	SendProxyRegister(hostID, hostname, ip string, capabilities []string) error
	SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error
	SendProxyMetrics(hostID string, metrics []byte) error
}

// Gateway manages tunnel and proxy subsystems for jump-host functionality.
type Gateway struct {
	cfg    Config
	logger zerolog.Logger

	tunnelSender TunnelSender
	proxySender  ProxySender

	listener net.Listener
	pool     *TunnelPool
	proxyMgr *ProxyManager

	mu       sync.RWMutex
	running  bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a Gateway. Call Start to begin accepting connections.
func New(cfg Config, logger zerolog.Logger, tunnelSender TunnelSender, proxySender ProxySender) *Gateway {
	return &Gateway{
		cfg:          cfg,
		logger:       logger.With().Str("component", "gateway").Logger(),
		tunnelSender: tunnelSender,
		proxySender:  proxySender,
		pool:         NewTunnelPool(cfg.MaxTunnels),
	}
}

// Start begins the gateway listener and background routines.
func (g *Gateway) Start(ctx context.Context) error {
	g.ctx, g.cancel = context.WithCancel(ctx)

	if g.cfg.ListenAddr != "" {
		ln, err := net.Listen("tcp", g.cfg.ListenAddr)
		if err != nil {
			return fmt.Errorf("gateway listen %s: %w", g.cfg.ListenAddr, err)
		}
		g.listener = ln
		g.logger.Info().Str("addr", g.cfg.ListenAddr).Msg("gateway listener started")

		g.wg.Add(1)
		go g.acceptLoop()
	}

	g.wg.Add(1)
	go g.idleReaper()

	// Register proxy hosts.
	for _, h := range g.cfg.Hosts {
		if h.Mode == "proxy" || h.Mode == "auto" {
			if err := g.proxySender.SendProxyRegister(h.ID, h.ID, h.Addr, nil); err != nil {
				g.logger.Warn().Err(err).Str("host_id", h.ID).Msg("failed to register proxy host")
			}
		}
	}

	g.mu.Lock()
	g.running = true
	g.mu.Unlock()

	g.logger.Info().Int("hosts", len(g.cfg.Hosts)).Msg("gateway started")
	return nil
}

// Stop gracefully shuts down the gateway.
func (g *Gateway) Stop(ctx context.Context) error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
	}
	g.running = false
	g.mu.Unlock()

	if g.cancel != nil {
		g.cancel()
	}

	if g.listener != nil {
		g.listener.Close()
	}

	g.pool.CloseAll()

	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	g.logger.Info().Msg("gateway stopped")
	return nil
}

// HealthStatus reports gateway health.
func (g *Gateway) HealthStatus() health.Status {
	g.mu.RLock()
	running := g.running
	g.mu.RUnlock()

	if !running {
		return health.Status{Status: "stopped"}
	}

	active := g.pool.ActiveCount()
	return health.Status{
		Status: "running",
		Details: map[string]any{
			"active_tunnels": active,
			"max_tunnels":    g.cfg.MaxTunnels,
		},
	}
}

// HandleTunnelData processes tunnel data from the platform.
func (g *Gateway) HandleTunnelData(tunnelID string, data []byte) error {
	t := g.pool.Get(tunnelID)
	if t == nil {
		return fmt.Errorf("tunnel %s not found", tunnelID)
	}
	return t.SendToTarget(data)
}

// HandleTunnelClose processes tunnel close from the platform.
func (g *Gateway) HandleTunnelClose(tunnelID, reason string) error {
	t := g.pool.Remove(tunnelID)
	if t == nil {
		return nil
	}
	g.logger.Info().Str("tunnel_id", tunnelID).Str("reason", reason).Msg("tunnel closed by platform")
	return t.Close()
}

// HandleProxyCommand processes a proxy command from the platform.
func (g *Gateway) HandleProxyCommand(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error {
	var hostCfg *HostConfig
	for _, h := range g.cfg.Hosts {
		if h.ID == hostID {
			hostCfg = &h
			break
		}
	}
	if hostCfg == nil {
		return fmt.Errorf("proxy host %s not configured", hostID)
	}

	g.logger.Info().Str("host_id", hostID).Str("command", command).Msg("proxy command received")

	start := time.Now()
	exitCode, stdout, stderr, timedOut := g.executeProxyCommand(ctx, *hostCfg, command, args, timeoutSec)
	duration := time.Since(start)

	return g.proxySender.SendProxyResponse(hostID, command, exitCode, stdout, stderr, duration, timedOut)
}

func (g *Gateway) acceptLoop() {
	defer g.wg.Done()
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			select {
			case <-g.ctx.Done():
				return
			default:
				g.logger.Error().Err(err).Msg("gateway accept error")
				continue
			}
		}
		g.wg.Add(1)
		go g.handleIncoming(conn)
	}
}

func (g *Gateway) handleIncoming(conn net.Conn) {
	defer g.wg.Done()
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	g.logger.Info().Str("remote", remoteAddr).Msg("incoming connection")

	// Read initial bytes to determine if this is an agent connection.
	// For now, treat all incoming connections as tunnel candidates.
	tunnelID := fmt.Sprintf("tunnel-%d", time.Now().UnixNano())

	t, err := NewTunnel(tunnelID, conn, g.tunnelSender, g.cfg.TunnelTimeout, g.cfg.IdleTimeout)
	if err != nil {
		g.logger.Error().Err(err).Str("remote", remoteAddr).Msg("failed to create tunnel")
		return
	}

	if !g.pool.Add(t) {
		g.logger.Warn().Str("remote", remoteAddr).Msg("max tunnels reached, rejecting")
		t.Close()
		return
	}

	g.logger.Info().Str("tunnel_id", tunnelID).Str("remote", remoteAddr).Msg("tunnel created")

	// Notify platform.
	if err := g.tunnelSender.SendTunnelOpen(tunnelID, "", "", remoteAddr, nil); err != nil {
		g.logger.Error().Err(err).Msg("failed to send tunnel open")
		g.pool.Remove(tunnelID)
		t.Close()
		return
	}

	// Run relay until tunnel closes.
	t.Relay(g.ctx)
	g.pool.Remove(tunnelID)
	g.logger.Info().Str("tunnel_id", tunnelID).Msg("tunnel relay ended")
}

func (g *Gateway) idleReaper() {
	defer g.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.pool.CloseIdle()
		}
	}
}

func (g *Gateway) executeProxyCommand(ctx context.Context, host HostConfig, command string, args []string, timeoutSec int32) (int, []byte, []byte, bool) {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := g.connectSSH(ctx, host)
	if err != nil {
		g.logger.Error().Err(err).Str("host_id", host.ID).Msg("ssh connect failed")
		return -1, nil, []byte(err.Error()), false
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return -1, nil, []byte(err.Error()), false
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	fullCmd := command
	for _, arg := range args {
		fullCmd += " " + arg
	}

	err = session.Run(fullCmd)
	exitCode := 0
	timedOut := false
	if err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else if ctx.Err() == context.DeadlineExceeded {
			timedOut = true
			exitCode = -1
		} else {
			exitCode = -1
		}
	}

	return exitCode, stdout.Bytes(), stderr.Bytes(), timedOut
}
```

Note: The `connectSSH` method will be implemented in Task 6 (proxy/ssh.go). For now, add a stub:

```go
func (g *Gateway) connectSSH(ctx context.Context, host HostConfig) (*ssh.Client, error) {
	// Implemented in proxy/ssh.go
	return nil, fmt.Errorf("not implemented")
}
```

- [ ] **Step 3: Add missing imports**

Ensure these imports are in `gateway.go`:

```go
import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"

	"github.com/cy77cc/opsagent/internal/health"
)
```

- [ ] **Step 4: Run build check**

Run:
```bash
go build ./internal/gateway/...
```

Expected: Compiles (with stub).

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/
git commit -m "feat(gateway): add core gateway module with lifecycle"
```

---

## Task 4: Tunnel Pool

**Files:**
- Create: `internal/gateway/tunnel/pool.go`
- Create: `internal/gateway/tunnel/pool_test.go`

- [ ] **Step 1: Create tunnel pool**

Create `internal/gateway/tunnel/pool.go`:

```go
package tunnel

import (
	"sync"
)

// Pool manages active tunnels with a maximum limit.
type Pool struct {
	mu       sync.RWMutex
	tunnels  map[string]*Tunnel
	maxCount int
}

// NewPool creates a Pool with the given maximum tunnel count.
func NewPool(maxCount int) *Pool {
	return &Pool{
		tunnels:  make(map[string]*Tunnel),
		maxCount: maxCount,
	}
}

// Add inserts a tunnel. Returns false if at capacity.
func (p *Pool) Add(t *Tunnel) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tunnels) >= p.maxCount {
		return false
	}
	p.tunnels[t.ID()] = t
	return true
}

// Get returns the tunnel with the given ID, or nil.
func (p *Pool) Get(id string) *Tunnel {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tunnels[id]
}

// Remove removes and returns the tunnel with the given ID, or nil.
func (p *Pool) Remove(id string) *Tunnel {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.tunnels[id]
	delete(p.tunnels, id)
	return t
}

// ActiveCount returns the number of active tunnels.
func (p *Pool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tunnels)
}

// CloseIdle closes tunnels that have been idle beyond their timeout.
func (p *Pool) CloseIdle() {
	p.mu.Lock()
	var toClose []*Tunnel
	for id, t := range p.tunnels {
		if t.IsIdle() {
			toClose = append(toClose, t)
			delete(p.tunnels, id)
		}
	}
	p.mu.Unlock()

	for _, t := range toClose {
		t.Close()
	}
}

// CloseAll closes all tunnels.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	tunnels := make([]*Tunnel, 0, len(p.tunnels))
	for _, t := range p.tunnels {
		tunnels = append(tunnels, t)
	}
	p.tunnels = make(map[string]*Tunnel)
	p.mu.Unlock()

	for _, t := range tunnels {
		t.Close()
	}
}
```

- [ ] **Step 2: Write pool tests**

Create `internal/gateway/tunnel/pool_test.go`:

```go
package tunnel

import (
	"testing"
	"time"
)

func TestPoolAdd(t *testing.T) {
	p := NewPool(2)
	t1 := &Tunnel{id: "a"}
	t2 := &Tunnel{id: "b"}
	t3 := &Tunnel{id: "c"}

	if !p.Add(t1) {
		t.Fatal("expected add t1 to succeed")
	}
	if !p.Add(t2) {
		t.Fatal("expected add t2 to succeed")
	}
	if p.Add(t3) {
		t.Fatal("expected add t3 to fail (at capacity)")
	}
}

func TestPoolGet(t *testing.T) {
	p := NewPool(10)
	t1 := &Tunnel{id: "x"}
	p.Add(t1)

	if got := p.Get("x"); got != t1 {
		t.Fatalf("expected t1, got %v", got)
	}
	if got := p.Get("missing"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestPoolRemove(t *testing.T) {
	p := NewPool(10)
	t1 := &Tunnel{id: "x"}
	p.Add(t1)

	removed := p.Remove("x")
	if removed != t1 {
		t.Fatalf("expected t1, got %v", removed)
	}
	if p.Get("x") != nil {
		t.Fatal("expected tunnel to be removed")
	}
	if p.ActiveCount() != 0 {
		t.Fatalf("expected 0, got %d", p.ActiveCount())
	}
}

func TestPoolActiveCount(t *testing.T) {
	p := NewPool(10)
	p.Add(&Tunnel{id: "a"})
	p.Add(&Tunnel{id: "b"})
	if p.ActiveCount() != 2 {
		t.Fatalf("expected 2, got %d", p.ActiveCount())
	}
}

func TestPoolCloseAll(t *testing.T) {
	p := NewPool(10)
	p.Add(&Tunnel{id: "a"})
	p.Add(&Tunnel{id: "b"})
	p.CloseAll()
	if p.ActiveCount() != 0 {
		t.Fatalf("expected 0 after CloseAll, got %d", p.ActiveCount())
	}
}

func TestPoolCloseIdle(t *testing.T) {
	p := NewPool(10)
	t1 := &Tunnel{id: "idle", lastActivity: time.Now().Add(-10 * time.Minute)}
	t2 := &Tunnel{id: "active", lastActivity: time.Now()}
	p.Add(t1)
	p.Add(t2)

	p.CloseIdle()

	if p.Get("idle") != nil {
		t.Fatal("expected idle tunnel to be closed")
	}
	if p.Get("active") == nil {
		t.Fatal("expected active tunnel to remain")
	}
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
go test ./internal/gateway/tunnel/... -v -race
```

Expected: All pool tests pass. Tunnel tests may fail (Tunnel type not yet complete).

- [ ] **Step 4: Commit**

```bash
git add internal/gateway/tunnel/
git commit -m "feat(gateway): add tunnel pool with capacity management"
```

---

## Task 5: Tunnel Implementation

**Files:**
- Create: `internal/gateway/tunnel/tunnel.go`
- Create: `internal/gateway/tunnel/tunnel_test.go`

- [ ] **Step 1: Create tunnel type**

Create `internal/gateway/tunnel/tunnel.go`:

```go
package tunnel

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// Sender sends tunnel data to the platform.
type Sender interface {
	SendTunnelData(tunnelID string, payload []byte) error
	SendTunnelClose(tunnelID, reason string) error
}

// Tunnel bridges a TCP connection with the platform via gRPC tunnel messages.
type Tunnel struct {
	id           string
	conn         net.Conn
	sender       Sender
	tunnelTimeout time.Duration
	idleTimeout  time.Duration

	mu           sync.Mutex
	lastActivity time.Time
	closed       bool
	done         chan struct{}
}

// NewTunnel creates a Tunnel wrapping the given TCP connection.
func NewTunnel(id string, conn net.Conn, sender Sender, tunnelTimeout, idleTimeout time.Duration) (*Tunnel, error) {
	return &Tunnel{
		id:            id,
		conn:          conn,
		sender:        sender,
		tunnelTimeout: tunnelTimeout,
		idleTimeout:   idleTimeout,
		lastActivity:  time.Now(),
		done:          make(chan struct{}),
	}, nil
}

// ID returns the tunnel identifier.
func (t *Tunnel) ID() string { return t.id }

// IsIdle reports whether the tunnel has exceeded its idle timeout.
func (t *Tunnel) IsIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.lastActivity) > t.idleTimeout
}

// SendToTarget writes data from the platform to the TCP connection.
func (t *Tunnel) SendToTarget(data []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return io.ErrClosedPipe
	}
	t.lastActivity = time.Now()
	t.mu.Unlock()

	t.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := t.conn.Write(data)
	return err
}

// Close shuts down the tunnel.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.done)
	t.mu.Unlock()

	t.sender.SendTunnelClose(t.id, "closed")
	return t.conn.Close()
}

// Relay reads from the TCP connection and sends to platform until context cancelled or connection closed.
func (t *Tunnel) Relay(ctx context.Context) {
	defer t.conn.Close()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			t.Close()
			return
		case <-t.done:
			return
		default:
		}

		t.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := t.conn.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.lastActivity = time.Now()
			t.mu.Unlock()

			payload := make([]byte, n)
			copy(payload, buf[:n])

			if sendErr := t.sender.SendTunnelData(t.id, payload); sendErr != nil {
				t.Close()
				return
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // read timeout, loop back to check context
			}
			if err != io.EOF {
				t.Close()
			}
			return
		}
	}
}
```

- [ ] **Step 2: Write tunnel tests**

Create `internal/gateway/tunnel/tunnel_test.go`:

```go
package tunnel

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

type mockSender struct {
	mu       sync.Mutex
	dataMsgs []struct{ id string; data []byte }
	closeMsgs []struct{ id, reason string }
}

func (m *mockSender) SendTunnelData(id string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := make([]byte, len(payload))
	copy(data, payload)
	m.dataMsgs = append(m.dataMsgs, struct{ id string; data []byte }{id, data})
	return nil
}

func (m *mockSender) SendTunnelClose(id, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeMsgs = append(m.closeMsgs, struct{ id, reason string }{id, reason})
	return nil
}

func (m *mockSender) DataCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dataMsgs)
}

func TestTunnelRelay(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	sender := &mockSender{}
	tun, err := NewTunnel("test-1", server, sender, 5*time.Second, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tun.Relay(ctx)

	// Write data from the "C agent" side.
	go func() {
		client.Write([]byte("hello"))
		time.Sleep(50 * time.Millisecond)
		client.Write([]byte("world"))
		time.Sleep(50 * time.Millisecond)
		cancel() // end relay
	}()

	// Wait for relay to finish.
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond)

	if sender.DataCount() < 1 {
		t.Fatal("expected at least 1 data message")
	}
}

func TestTunnelSendToTarget(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	sender := &mockSender{}
	tun, err := NewTunnel("test-2", server, sender, 5*time.Second, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		buf := make([]byte, 1024)
		n, _ := client.Read(buf)
		if string(buf[:n]) != "from-platform" {
			t.Errorf("expected 'from-platform', got %q", string(buf[:n]))
		}
	}()

	if err := tun.SendToTarget([]byte("from-platform")); err != nil {
		t.Fatal(err)
	}
}

func TestTunnelIsIdle(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	sender := &mockSender{}
	tun, err := NewTunnel("test-3", server, sender, 5*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	if tun.IsIdle() {
		t.Fatal("should not be idle immediately")
	}

	tun.lastActivity = time.Now().Add(-200 * time.Millisecond)
	if !tun.IsIdle() {
		t.Fatal("should be idle after timeout")
	}
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
go test ./internal/gateway/tunnel/... -v -race
```

Expected: All tunnel and pool tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/gateway/tunnel/
git commit -m "feat(gateway): add tunnel TCP-gRPC relay implementation"
```

---

## Task 6: SSH Proxy Client

**Files:**
- Create: `internal/gateway/proxy/ssh.go`
- Create: `internal/gateway/proxy/ssh_test.go`

- [ ] **Step 1: Create SSH client**

Create `internal/gateway/proxy/ssh.go`:

```go
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHConfig holds SSH connection parameters.
type SSHConfig struct {
	User     string
	Password string
	KeyFile  string
	Port     int
}

// SSHClient manages SSH connections to internal hosts.
type SSHClient struct {
	cfg SSHConfig
}

// NewSSHClient creates an SSHClient.
func NewSSHClient(cfg SSHConfig) *SSHClient {
	return &SSHClient{cfg: cfg}
}

// Connect establishes an SSH connection to the given address.
func (c *SSHClient) Connect(ctx context.Context, addr string) (*ssh.Client, error) {
	auth, err := c.buildAuth()
	if err != nil {
		return nil, fmt.Errorf("ssh auth: %w", err)
	}

	addr = fmt.Sprintf("%s:%d", addr, c.cfg.Port)

	config := &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	// Use dialer with context.
	conn, err := (&net.Dialer{Timeout: config.Timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// Execute runs a command on an SSH client and returns the result.
func (c *SSHClient) Execute(ctx context.Context, client *ssh.Client, command string, args []string) (exitCode int, stdout, stderr []byte, timedOut bool) {
	session, err := client.NewSession()
	if err != nil {
		return -1, nil, []byte(err.Error()), false
	}
	defer session.Close()

	var outBuf, errBuf bytes.Buffer
	session.Stdout = &outBuf
	session.Stderr = &errBuf

	fullCmd := command
	for _, arg := range args {
		fullCmd += " " + arg
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Run(fullCmd)
	}()

	select {
	case <-ctx.Done():
		session.Signal(ssh.SIGKILL)
		return -1, outBuf.Bytes(), errBuf.Bytes(), true
	case err := <-done:
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				return exitErr.ExitStatus(), outBuf.Bytes(), errBuf.Bytes(), false
			}
			return -1, outBuf.Bytes(), []byte(err.Error()), false
		}
		return 0, outBuf.Bytes(), errBuf.Bytes(), false
	}
}

func (c *SSHClient) buildAuth() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if c.cfg.Password != "" {
		methods = append(methods, ssh.Password(c.cfg.Password))
	}

	if c.cfg.KeyFile != "" {
		key, err := os.ReadFile(c.cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no ssh auth methods configured (set password or key_file)")
	}

	return methods, nil
}
```

Note: Add `"net"` to imports.

- [ ] **Step 2: Write SSH client tests**

Create `internal/gateway/proxy/ssh_test.go`:

```go
package proxy

import (
	"testing"
)

func TestSSHClientBuildAuthPassword(t *testing.T) {
	c := NewSSHClient(SSHConfig{User: "root", Password: "pass", Port: 22})
	methods, err := c.buildAuth()
	if err != nil {
		t.Fatal(err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected 1 auth method, got %d", len(methods))
	}
}

func TestSSHClientBuildAuthNoMethods(t *testing.T) {
	c := NewSSHClient(SSHConfig{User: "root", Port: 22})
	_, err := c.buildAuth()
	if err == nil {
		t.Fatal("expected error for no auth methods")
	}
}

func TestSSHClientBuildAuthKeyFile(t *testing.T) {
	// Test with non-existent key file.
	c := NewSSHClient(SSHConfig{User: "root", KeyFile: "/nonexistent/key", Port: 22})
	_, err := c.buildAuth()
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
go test ./internal/gateway/proxy/... -v -race
```

Expected: All SSH tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/gateway/proxy/
git commit -m "feat(gateway): add SSH proxy client for remote execution"
```

---

## Task 7: Proxy Manager

**Files:**
- Create: `internal/gateway/proxy/proxy.go`
- Create: `internal/gateway/proxy/proxy_test.go`

- [ ] **Step 1: Create proxy manager**

Create `internal/gateway/proxy/proxy.go`:

```go
package proxy

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"

	"github.com/cy77cc/opsagent/internal/health"
)

// Sender sends proxy responses to the platform.
type Sender interface {
	SendProxyRegister(hostID, hostname, ip string, capabilities []string) error
	SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error
	SendProxyMetrics(hostID string, metrics []byte) error
}

// HostConfig defines a proxy host.
type HostConfig struct {
	ID   string
	Addr string
	SSH  SSHConfig
}

// Manager handles proxy mode: executing commands on behalf of internal hosts.
type Manager struct {
	hosts  map[string]HostConfig
	sender Sender
	logger zerolog.Logger
	sshClients map[string]*SSHClient
}

// NewManager creates a proxy Manager.
func NewManager(hosts []HostConfig, sender Sender, logger zerolog.Logger) *Manager {
	m := &Manager{
		hosts:      make(map[string]HostConfig),
		sender:     sender,
		logger:     logger.With().Str("component", "proxy").Logger(),
		sshClients: make(map[string]*SSHClient),
	}
	for _, h := range hosts {
		m.hosts[h.ID] = h
		m.sshClients[h.ID] = NewSSHClient(h.SSH)
	}
	return m
}

// RegisterHosts sends proxy host registrations to the platform.
func (m *Manager) RegisterHosts() error {
	for _, h := range m.hosts {
		if err := m.sender.SendProxyRegister(h.ID, h.ID, h.Addr, nil); err != nil {
			m.logger.Warn().Err(err).Str("host_id", h.ID).Msg("failed to register proxy host")
			return err
		}
		m.logger.Info().Str("host_id", h.ID).Str("addr", h.Addr).Msg("proxy host registered")
	}
	return nil
}

// ExecuteCommand runs a command on a proxy host via SSH.
func (m *Manager) ExecuteCommand(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error {
	host, ok := m.hosts[hostID]
	if !ok {
		return fmt.Errorf("proxy host %s not configured", hostID)
	}

	client, ok := m.sshClients[hostID]
	if !ok {
		return fmt.Errorf("ssh client for %s not found", hostID)
	}

	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m.logger.Info().Str("host_id", hostID).Str("command", command).Msg("executing proxy command")
	start := time.Now()

	sshConn, err := client.Connect(ctx, host.Addr)
	if err != nil {
		m.logger.Error().Err(err).Str("host_id", hostID).Msg("ssh connect failed")
		return m.sender.SendProxyResponse(hostID, command, -1, nil, []byte(err.Error()), time.Since(start), false)
	}
	defer sshConn.Close()

	exitCode, stdout, stderr, timedOut := client.Execute(ctx, sshConn, command, args)
	duration := time.Since(start)

	m.logger.Info().Str("host_id", hostID).Int("exit_code", exitCode).Dur("duration", duration).Msg("proxy command completed")
	return m.sender.SendProxyResponse(hostID, command, exitCode, stdout, stderr, duration, timedOut)
}

// HealthStatus reports proxy manager health.
func (m *Manager) HealthStatus() health.Status {
	return health.Status{
		Status: "running",
		Details: map[string]any{
			"proxy_hosts": len(m.hosts),
		},
	}
}

// ExecuteMetricsCollect collects metrics from a proxy host via SSH.
func (m *Manager) ExecuteMetricsCollect(ctx context.Context, hostID string) error {
	host, ok := m.hosts[hostID]
	if !ok {
		return fmt.Errorf("proxy host %s not configured", hostID)
	}

	client, ok := m.sshClients[hostID]
	if !ok {
		return fmt.Errorf("ssh client for %s not found", hostID)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sshConn, err := client.Connect(ctx, host.Addr)
	if err != nil {
		return err
	}
	defer sshConn.Close()

	// Collect basic system metrics via SSH commands.
	commands := map[string]string{
		"cpu":    "top -bn1 | head -5",
		"memory": "free -b",
		"disk":   "df -B1",
		"load":   "cat /proc/loadavg",
	}

	metrics := make(map[string]string)
	for name, cmd := range commands {
		_, stdout, _, _ := client.Execute(ctx, sshConn, cmd, nil)
		metrics[name] = string(stdout)
	}

	// TODO: Parse metrics into proper format and send via sender.SendProxyMetrics
	_ = metrics
	return nil
}
```

Note: Add `"github.com/cy77cc/opsagent/internal/health"` to imports. The `bytes` import is not needed here (remove it).

- [ ] **Step 2: Write proxy manager tests**

Create `internal/gateway/proxy/proxy_test.go`:

```go
package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type mockSender struct {
	registers []struct{ hostID, hostname, ip string }
	responses []struct {
		hostID, command string
		exitCode        int
	}
}

func (m *mockSender) SendProxyRegister(hostID, hostname, ip string, capabilities []string) error {
	m.registers = append(m.registers, struct{ hostID, hostname, ip string }{hostID, hostname, ip})
	return nil
}

func (m *mockSender) SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error {
	m.responses = append(m.responses, struct {
		hostID, command string
		exitCode        int
	}{hostID, command, exitCode})
	return nil
}

func (m *mockSender) SendProxyMetrics(hostID string, metrics []byte) error {
	return nil
}

func TestManagerRegisterHosts(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	hosts := []HostConfig{
		{ID: "c1", Addr: "192.168.1.10", SSH: SSHConfig{User: "root", Password: "pass", Port: 22}},
		{ID: "c2", Addr: "192.168.1.11", SSH: SSHConfig{User: "root", Password: "pass", Port: 22}},
	}

	m := NewManager(hosts, sender, logger)
	if err := m.RegisterHosts(); err != nil {
		t.Fatal(err)
	}

	if len(sender.registers) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(sender.registers))
	}
}

func TestManagerExecuteCommandUnknownHost(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	m := NewManager(nil, sender, logger)

	err := m.ExecuteCommand(context.Background(), "unknown", "uptime", nil, 10)
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
}

func TestManagerHealthStatus(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	hosts := []HostConfig{
		{ID: "c1", Addr: "192.168.1.10", SSH: SSHConfig{User: "root", Password: "pass", Port: 22}},
	}
	m := NewManager(hosts, sender, logger)

	st := m.HealthStatus()
	if st.Status != "running" {
		t.Fatalf("expected running, got %s", st.Status)
	}
}
```

- [ ] **Step 3: Run tests**

Run:
```bash
go test ./internal/gateway/proxy/... -v -race
```

Expected: All proxy tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/gateway/proxy/
git commit -m "feat(gateway): add proxy manager for SSH-based command execution"
```

---

## Task 8: Wire SSH into Gateway

**Files:**
- Modify: `internal/gateway/gateway.go`

- [ ] **Step 1: Replace SSH stub with real implementation**

In `internal/gateway/gateway.go`, replace the `connectSSH` stub and `executeProxyCommand` with the real implementation using the proxy package.

Replace the `executeProxyCommand` method:

```go
func (g *Gateway) executeProxyCommand(ctx context.Context, host HostConfig, command string, args []string, timeoutSec int32) (int, []byte, []byte, bool) {
	sshClient := proxy.NewSSHClient(proxy.SSHConfig{
		User:     host.SSH.User,
		Password: host.SSH.Password,
		KeyFile:  host.SSH.KeyFile,
		Port:     host.SSH.Port,
	})

	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := sshClient.Connect(ctx, host.Addr)
	if err != nil {
		g.logger.Error().Err(err).Str("host_id", host.ID).Msg("ssh connect failed")
		return -1, nil, []byte(err.Error()), false
	}
	defer conn.Close()

	exitCode, stdout, stderr, timedOut := sshClient.Execute(ctx, conn, command, args)
	return exitCode, stdout, stderr, timedOut
}
```

Remove the `connectSSH` stub method entirely.

Update imports to add:
```go
"github.com/cy77cc/opsagent/internal/gateway/proxy"
```

Remove the unused `"golang.org/x/crypto/ssh"` and `"bytes"` imports from gateway.go (they're now in the proxy package).

- [ ] **Step 2: Verify build**

Run:
```bash
go build ./internal/gateway/...
```

Expected: Compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/gateway/gateway.go
git commit -m "feat(gateway): wire SSH proxy client into gateway module"
```

---

## Task 9: gRPC Receiver Integration

**Files:**
- Modify: `internal/grpcclient/receiver.go`
- Modify: `internal/grpcclient/sender.go`

- [ ] **Step 1: Add tunnel/proxy handler types to receiver**

Add to `internal/grpcclient/receiver.go` after the existing handler types:

```go
// TunnelDataHandler handles tunnel data from the platform.
type TunnelDataHandler func(ctx context.Context, tunnelID string, data []byte) error

// TunnelCloseHandler handles tunnel close from the platform.
type TunnelCloseHandler func(ctx context.Context, tunnelID, reason string) error

// ProxyCommandHandler handles proxy command requests from the platform.
type ProxyCommandHandler func(ctx context.Context, hostID, command string, args []string, timeoutSeconds int32) error
```

Add handler fields to the `Receiver` struct:

```go
onTunnelData  TunnelDataHandler
onTunnelClose TunnelCloseHandler
onProxyCmd    ProxyCommandHandler
```

Add setter methods:

```go
// SetTunnelDataHandler registers the handler for tunnel data messages.
func (r *Receiver) SetTunnelDataHandler(h TunnelDataHandler) { r.onTunnelData = h }

// SetTunnelCloseHandler registers the handler for tunnel close messages.
func (r *Receiver) SetTunnelCloseHandler(h TunnelCloseHandler) { r.onTunnelClose = h }

// SetProxyCommandHandler registers the handler for proxy command messages.
func (r *Receiver) SetProxyCommandHandler(h ProxyCommandHandler) { r.onProxyCmd = h }
```

Add cases to the `Handle` type switch (before the `default` case):

```go
case *pb.PlatformMessage_TunnelData:
	r.logger.Info().Str("tunnel_id", p.TunnelData.GetTunnelId()).Int("bytes", len(p.TunnelData.GetPayload())).Msg("received TunnelData")
	if r.onTunnelData != nil {
		return r.onTunnelData(ctx, p.TunnelData.GetTunnelId(), p.TunnelData.GetPayload())
	}
	r.logger.Warn().Str("tunnel_id", p.TunnelData.GetTunnelId()).Msg("no tunnel data handler registered")

case *pb.PlatformMessage_TunnelClose:
	r.logger.Info().Str("tunnel_id", p.TunnelClose.GetTunnelId()).Str("reason", p.TunnelClose.GetReason()).Msg("received TunnelClose")
	if r.onTunnelClose != nil {
		return r.onTunnelClose(ctx, p.TunnelClose.GetTunnelId(), p.TunnelClose.GetReason())
	}
	r.logger.Warn().Str("tunnel_id", p.TunnelClose.GetTunnelId()).Msg("no tunnel close handler registered")

case *pb.PlatformMessage_ProxyCommand:
	r.logger.Info().Str("host_id", p.ProxyCommand.GetHostId()).Str("command", p.ProxyCommand.GetCommand()).Msg("received ProxyCommand")
	if r.onProxyCmd != nil {
		return r.onProxyCmd(ctx, p.ProxyCommand.GetHostId(), p.ProxyCommand.GetCommand(), p.ProxyCommand.GetArgs(), p.ProxyCommand.GetTimeoutSeconds())
	}
	r.logger.Warn().Str("host_id", p.ProxyCommand.GetHostId()).Msg("no proxy command handler registered")
```

- [ ] **Step 2: Add tunnel/proxy sender methods**

Add to `internal/grpcclient/sender.go`:

```go
// NewTunnelOpenMessage creates a tunnel-open AgentMessage.
func NewTunnelOpenMessage(tunnelID, agentID, hostname, ip string, capabilities []string) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_TunnelOpen{
			TunnelOpen: &pb.TunnelOpen{
				TunnelId:     tunnelID,
				AgentId:      agentID,
				Hostname:     hostname,
				Ip:           ip,
				Capabilities: capabilities,
			},
		},
	}
}

// NewTunnelDataMessage creates a tunnel-data AgentMessage.
func NewTunnelDataMessage(tunnelID string, payload []byte) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_TunnelData{
			TunnelData: &pb.TunnelData{
				TunnelId: tunnelID,
				Payload:  payload,
			},
		},
	}
}

// NewTunnelCloseMessage creates a tunnel-close AgentMessage.
func NewTunnelCloseMessage(tunnelID, reason string) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_TunnelClose{
			TunnelClose: &pb.TunnelClose{
				TunnelId: tunnelID,
				Reason:   reason,
			},
		},
	}
}

// NewProxyRegisterMessage creates a proxy-host-register AgentMessage.
func NewProxyRegisterMessage(hostID, hostname, ip string, capabilities []string) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_ProxyRegister{
			ProxyRegister: &pb.ProxyHostRegister{
				HostId:       hostID,
				Hostname:     hostname,
				Ip:           ip,
				Capabilities: capabilities,
			},
		},
	}
}

// NewProxyResponseMessage creates a proxy-command-response AgentMessage.
func NewProxyResponseMessage(hostID, command string, exitCode int, stdout, stderr []byte, durationMS int64, timedOut bool) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_ProxyResponse{
			ProxyResponse: &pb.ProxyCommandResponse{
				HostId:      hostID,
				Command:     command,
				ExitCode:    int32(exitCode),
				Stdout:      stdout,
				Stderr:      stderr,
				DurationMs:  durationMS,
				TimedOut:    timedOut,
			},
		},
	}
}

// NewProxyMetricsMessage creates a proxy-metric-batch AgentMessage.
func NewProxyMetricsMessage(hostID string, metrics *pb.MetricBatch) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_ProxyMetrics{
			ProxyMetrics: &pb.ProxyMetricBatch{
				HostId:  hostID,
				Metrics: metrics.Metrics,
			},
		},
	}
}
```

- [ ] **Step 3: Add stream-based send methods to gRPC Client**

Add to `internal/grpcclient/client.go`:

```go
// SendTunnelOpen sends a tunnel-open message.
func (c *Client) SendTunnelOpen(tunnelID, agentID, hostname, ip string, capabilities []string) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()
	if !connected || stream == nil {
		return fmt.Errorf("not connected")
	}
	return stream.Send(NewTunnelOpenMessage(tunnelID, agentID, hostname, ip, capabilities))
}

// SendTunnelData sends tunnel data.
func (c *Client) SendTunnelData(tunnelID string, payload []byte) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()
	if !connected || stream == nil {
		return fmt.Errorf("not connected")
	}
	return stream.Send(NewTunnelDataMessage(tunnelID, payload))
}

// SendTunnelClose sends a tunnel close message.
func (c *Client) SendTunnelClose(tunnelID, reason string) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()
	if !connected || stream == nil {
		return fmt.Errorf("not connected")
	}
	return stream.Send(NewTunnelCloseMessage(tunnelID, reason))
}

// SendProxyRegister sends a proxy host registration.
func (c *Client) SendProxyRegister(hostID, hostname, ip string, capabilities []string) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()
	if !connected || stream == nil {
		return fmt.Errorf("not connected")
	}
	return stream.Send(NewProxyRegisterMessage(hostID, hostname, ip, capabilities))
}

// SendProxyResponse sends a proxy command response.
func (c *Client) SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()
	if !connected || stream == nil {
		return fmt.Errorf("not connected")
	}
	return stream.Send(NewProxyResponseMessage(hostID, command, exitCode, stdout, stderr, duration.Milliseconds(), timedOut))
}

// SendProxyMetrics sends proxy-collected metrics.
func (c *Client) SendProxyMetrics(hostID string, metrics []byte) error {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()
	if !connected || stream == nil {
		return fmt.Errorf("not connected")
	}
	// For now, send as raw bytes. Metrics parsing will be added later.
	return nil
}
```

- [ ] **Step 4: Verify build**

Run:
```bash
go build ./...
```

Expected: Compiles cleanly.

- [ ] **Step 5: Run existing tests**

Run:
```bash
go test ./internal/grpcclient/... -v -race
```

Expected: All existing tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/grpcclient/
git commit -m "feat(grpc): add tunnel and proxy message handlers and senders"
```

---

## Task 10: App Wiring

**Files:**
- Modify: `internal/app/interfaces.go`
- Modify: `internal/app/agent.go`
- Modify: `internal/app/options.go`

- [ ] **Step 1: Add Gateway interface**

Add to `internal/app/interfaces.go`:

```go
// Gateway manages tunnel and proxy subsystems for jump-host functionality.
type Gateway interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HandleTunnelData(tunnelID string, data []byte) error
	HandleTunnelClose(tunnelID, reason string) error
	HandleProxyCommand(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error
	HealthStatus() health.Status
}
```

- [ ] **Step 2: Add WithGateway option**

Add to `internal/app/options.go`:

```go
// WithGateway injects a custom Gateway (for testing).
func WithGateway(gw Gateway) Option {
	return func(a *Agent) { a.gateway = gw }
}
```

- [ ] **Step 3: Add gateway field to Agent struct**

In `internal/app/agent.go`, add to the `Agent` struct:

```go
gateway        Gateway
```

- [ ] **Step 4: Wire gateway in NewAgent**

In `NewAgent`, after the checker executor block and before the HTTP server block, add:

```go
// Build gateway if enabled and not injected.
if a.gateway == nil && cfg.Gateway.Enabled {
	gwCfg := gateway.Config{
		ListenAddr:    cfg.Gateway.ListenAddr,
		MaxTunnels:    cfg.Gateway.MaxTunnels,
		TunnelTimeout: time.Duration(cfg.Gateway.TunnelTimeoutSeconds) * time.Second,
		IdleTimeout:   time.Duration(cfg.Gateway.IdleTimeoutSeconds) * time.Second,
	}
	for _, h := range cfg.Gateway.Hosts {
		gwCfg.Hosts = append(gwCfg.Hosts, gateway.HostConfig{
			ID:   h.ID,
			Addr: h.Addr,
			Mode: h.Mode,
			SSH: gateway.SSHConfig{
				User:     h.SSH.User,
				Password: h.SSH.Password,
				KeyFile:  h.SSH.KeyFile,
				Port:     h.SSH.Port,
			},
		})
	}
	a.gateway = gateway.New(gwCfg, log, nil, nil) // senders set after gRPC client creation
}
```

Note: The gateway needs the gRPC client as its sender. We need to set this after the gRPC client is created. Add a `SetSenders` method to Gateway, or pass the gRPC client later.

Actually, looking at the code flow, the gRPC client is created before the gateway. So we can pass it directly:

```go
if a.gateway == nil && cfg.Gateway.Enabled {
	// ... build gwCfg ...
	a.gateway = gateway.New(gwCfg, log, a.grpcClient, a.grpcClient)
}
```

But `a.grpcClient` is the `GRPCClient` interface which doesn't have the tunnel/proxy methods. We need to either:
1. Add those methods to the `GRPCClient` interface, or
2. Use a type assertion to get the concrete `*grpcclient.Client`

Option 1 is cleaner. Add to the `GRPCClient` interface:

```go
// Tunnel operations
SendTunnelOpen(tunnelID, agentID, hostname, ip string, capabilities []string) error
SendTunnelData(tunnelID string, payload []byte) error
SendTunnelClose(tunnelID, reason string) error
// Proxy operations
SendProxyRegister(hostID, hostname, ip string, capabilities []string) error
SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error
SendProxyMetrics(hostID string, metrics []byte) error
```

Then in `NewAgent`, create the gateway after the gRPC client:

```go
if a.gateway == nil && cfg.Gateway.Enabled {
	gwCfg := gateway.Config{
		ListenAddr:    cfg.Gateway.ListenAddr,
		MaxTunnels:    cfg.Gateway.MaxTunnels,
		TunnelTimeout: time.Duration(cfg.Gateway.TunnelTimeoutSeconds) * time.Second,
		IdleTimeout:   time.Duration(cfg.Gateway.IdleTimeoutSeconds) * time.Second,
	}
	for _, h := range cfg.Gateway.Hosts {
		gwCfg.Hosts = append(gwCfg.Hosts, gateway.HostConfig{
			ID:   h.ID,
			Addr: h.Addr,
			Mode: h.Mode,
			SSH: gateway.SSHConfig{
				User:     h.SSH.User,
				Password: h.SSH.Password,
				KeyFile:  h.SSH.KeyFile,
				Port:     h.SSH.Port,
			},
		})
	}
	a.gateway = gateway.New(gwCfg, log, a.grpcClient, a.grpcClient)
}
```

- [ ] **Step 5: Wire gateway into startSubsystems**

In `startSubsystems`, after the gRPC client start and before the HTTP server start:

```go
if a.gateway != nil {
	if err := a.gateway.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("start gateway: %w", err)
	}
	a.auditLog.Log(AuditEvent{
		EventType: "gateway.started", Component: "gateway",
		Action: "start", Status: "success",
	})
}
```

- [ ] **Step 6: Wire gateway into shutdown**

In `shutdown`, after the plugin gateway stop and before the HTTP server shutdown:

```go
if a.gateway != nil {
	if err := a.gateway.Stop(stopCtx); err != nil {
		a.log.Error().Err(err).Msg("failed to stop gateway")
	}
}
```

- [ ] **Step 7: Wire gateway handlers into gRPC receiver**

In `registerGRPCHandlers`, add after the health check handler:

```go
// Gateway tunnel handlers.
if a.gateway != nil {
	recv.SetTunnelDataHandler(func(ctx context.Context, tunnelID string, data []byte) error {
		return a.gateway.HandleTunnelData(tunnelID, data)
	})
	recv.SetTunnelCloseHandler(func(ctx context.Context, tunnelID, reason string) error {
		return a.gateway.HandleTunnelClose(tunnelID, reason)
	})
	recv.SetProxyCommandHandler(func(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error {
		return a.gateway.HandleProxyCommand(ctx, hostID, command, args, timeoutSec)
	})
}
```

- [ ] **Step 8: Add import for gateway package**

Add to `internal/app/agent.go` imports:

```go
"github.com/cy77cc/opsagent/internal/gateway"
```

- [ ] **Step 9: Verify build**

Run:
```bash
go build ./...
```

Expected: Compiles cleanly.

- [ ] **Step 10: Run all tests**

Run:
```bash
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 11: Commit**

```bash
git add internal/app/
git commit -m "feat(app): wire gateway module into agent lifecycle"
```

---

## Task 11: Health Check and Metrics Integration

**Files:**
- Modify: `internal/app/metrics.go`
- Modify: `internal/server/server.go`
- Modify: `internal/server/handlers.go`

- [ ] **Step 1: Add gateway metrics to MetricsRegistry**

Add to `internal/app/metrics.go` in the `MetricsRegistry` struct:

```go
GatewayTunnelsActive  prometheus.Gauge
GatewayTunnelBytes    prometheus.Counter
GatewayTunnelErrors   prometheus.Counter
GatewayProxyRequests  prometheus.Counter
GatewayProxyLatency   prometheus.Histogram
```

Add to `NewMetricsRegistry` after the existing metric definitions:

```go
GatewayTunnelsActive: prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "opsagent_gateway_tunnels_active", Help: "Number of active gateway tunnels",
}),
GatewayTunnelBytes: prometheus.NewCounter(prometheus.CounterOpts{
	Name: "opsagent_gateway_tunnel_bytes_total", Help: "Total bytes tunneled",
}),
GatewayTunnelErrors: prometheus.NewCounter(prometheus.CounterOpts{
	Name: "opsagent_gateway_tunnel_errors_total", Help: "Total tunnel errors",
}),
GatewayProxyRequests: prometheus.NewCounter(prometheus.CounterOpts{
	Name: "opsagent_gateway_proxy_requests_total", Help: "Total proxy requests",
}),
GatewayProxyLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
	Name:    "opsagent_gateway_proxy_latency_seconds",
	Help:    "Proxy command execution latency",
	Buckets: prometheus.DefBuckets,
}),
```

Register them in `reg.MustRegister(...)`:

```go
reg.MustRegister(
	// ... existing metrics ...
	m.GatewayTunnelsActive, m.GatewayTunnelBytes, m.GatewayTunnelErrors,
	m.GatewayProxyRequests, m.GatewayProxyLatency,
)
```

- [ ] **Step 2: Add Gateway to HealthCheckers**

In `internal/server/server.go`, add to the `HealthCheckers` struct:

```go
Gateway health.Statuser
```

- [ ] **Step 3: Add gateway to /healthz handler**

In `internal/server/handlers.go`, add to the `entries` slice:

```go
{"gateway", s.healthCheckers.Gateway, false},
```

- [ ] **Step 4: Wire gateway health checker in NewAgent**

In `internal/app/agent.go`, in the `server.New(...)` call, add `Gateway` to `HealthCheckers`:

```go
HealthCheckers: server.HealthCheckers{
	GRPC:      a.grpcClient,
	Scheduler: a.scheduler,
	PluginRT:  a.pluginRuntime,
	Gateway:   a.gateway,
},
```

- [ ] **Step 5: Verify build**

Run:
```bash
go build ./...
```

Expected: Compiles cleanly.

- [ ] **Step 6: Run all tests**

Run:
```bash
go test ./... -race
```

Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/app/metrics.go internal/server/
git commit -m "feat: add gateway health check and Prometheus metrics"
```

---

## Task 12: Audit Logging

**Files:**
- Modify: `internal/gateway/gateway.go`

- [ ] **Step 1: Add audit logging to gateway**

The gateway should emit audit events for key operations. Since the gateway doesn't have direct access to the `AuditLogger`, we'll use the logger and add audit event emission at the app layer.

In `internal/gateway/gateway.go`, add audit event methods:

```go
// AuditEvent returns an audit event for the given action.
func (g *Gateway) AuditEvent(action, status string, details map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"event_type": "gateway." + action,
		"component":  "gateway",
		"action":     action,
		"status":     status,
		"details":    details,
	}
}
```

In `internal/app/agent.go`, add audit logging to the gateway handler wrappers:

```go
recv.SetTunnelDataHandler(func(ctx context.Context, tunnelID string, data []byte) error {
	return a.gateway.HandleTunnelData(tunnelID, data)
})
recv.SetTunnelCloseHandler(func(ctx context.Context, tunnelID, reason string) error {
	a.auditLog.Log(AuditEvent{
		EventType: "gateway.tunnel.close", Component: "gateway",
		Action: "tunnel_close", Status: "success",
		Details: map[string]interface{}{"tunnel_id": tunnelID, "reason": reason},
	})
	return a.gateway.HandleTunnelClose(tunnelID, reason)
})
recv.SetProxyCommandHandler(func(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error {
	a.auditLog.Log(AuditEvent{
		EventType: "gateway.proxy.exec", Component: "gateway",
		Action: "proxy_command", Status: "started",
		Details: map[string]interface{}{"host_id": hostID, "command": command},
	})
	return a.gateway.HandleProxyCommand(ctx, hostID, command, args, timeoutSec)
})
```

- [ ] **Step 2: Verify build**

Run:
```bash
go build ./...
```

Expected: Compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/app/agent.go internal/gateway/gateway.go
git commit -m "feat: add gateway audit logging"
```

---

## Task 13: End-to-End Verification

- [ ] **Step 1: Run full test suite**

Run:
```bash
make test-race
```

Expected: All tests pass with race detector.

- [ ] **Step 2: Run linter**

Run:
```bash
make lint
```

Expected: No lint errors.

- [ ] **Step 3: Run go vet**

Run:
```bash
go vet ./...
```

Expected: No issues.

- [ ] **Step 4: Verify config loads**

Create a test config and verify it loads:

```bash
go run ./cmd/agent validate --config ./configs/config.yaml
```

Expected: Config validates (gateway disabled by default).

- [ ] **Step 5: Build binary**

Run:
```bash
make build
```

Expected: Binary builds to `bin/opsagent`.

- [ ] **Step 6: Smoke test**

Run:
```bash
./scripts/smoke-test.sh
```

Expected: All smoke tests pass.

- [ ] **Step 7: Final commit**

```bash
git add -A
git commit -m "feat: complete gateway tunnel feature implementation"
```
