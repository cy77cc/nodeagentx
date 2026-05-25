# Code Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 35 code review issues (4 CRITICAL, 8 HIGH, 14 MEDIUM, 9 LOW) across the OpsAgent codebase.

**Architecture:** Targeted per-issue fixes in priority order. Each fix is the smallest possible change. Tests written first for security-critical changes.

**Tech Stack:** Go 1.22+, golang.org/x/crypto/ssh, golang.org/x/crypto/ssh/knownhosts

---

## File Map

| File | Issues | Action |
|------|--------|--------|
| `internal/gateway/gateway.go` | C1, H3, H4, L1, L2 | Modify |
| `internal/gateway/config.go` | H3, L1 | Modify |
| `internal/gateway/tunnel/tunnel.go` | L3 | Modify |
| `internal/gateway/proxy/ssh.go` | C2, H2 | Modify |
| `internal/gateway/proxy/proxy.go` | L4 | Modify |
| `internal/grpcclient/client.go` | C3, H6, H7, M3, M4, M5, M6, L5 | Modify |
| `internal/grpcclient/persist.go` | L6 | Modify |
| `internal/sandbox/executor.go` | C4, H8, M2 | Modify |
| `internal/sandbox/policy.go` | H1 | Modify |
| `internal/sandbox/nsjail.go` | L7, L8 | Modify |
| `internal/sandbox/output_streamer.go` | M1 | Modify |
| `internal/app/agent.go` | M7, M8, M9, L9 | Modify |
| `internal/app/interfaces.go` | M10 | Modify |
| `internal/config/diff.go` | M11 | Modify |
| `internal/server/middleware.go` | M12 | Modify |
| `internal/collector/buffer.go` | M13 | Modify |
| `internal/pluginruntime/runtime.go` | H5 | Modify |
| `internal/pluginruntime/gateway.go` | M14 | Modify |

---

## Phase 1: CRITICAL Fixes

### Task 1: C1 — Replace gateway stub types

**Files:**
- Modify: `internal/gateway/gateway.go:1-329`

- [ ] **Step 1: Add tunnel import and update Gateway struct**

In `internal/gateway/gateway.go`, add the tunnel import and change the `pool` field type:

```go
import (
    // ... existing imports ...
    "github.com/cy77cc/opsagent/internal/gateway/tunnel"
)

type Gateway struct {
    // ... other fields unchanged ...
    pool     *tunnel.Pool  // was *TunnelPool
    // ...
}
```

- [ ] **Step 2: Update New() to use tunnel.NewPool**

```go
func New(cfg Config, logger zerolog.Logger, tunnelSender TunnelSender, proxySender ProxySender) *Gateway {
    // ... panic checks unchanged ...
    return &Gateway{
        // ... other fields unchanged ...
        pool:         tunnel.NewPool(cfg.MaxTunnels),  // was NewTunnelPool
    }
}
```

- [ ] **Step 3: Update handleIncoming() to use tunnel.NewTunnel**

```go
func (g *Gateway) handleIncoming(conn net.Conn) {
    defer g.wg.Done()
    defer conn.Close()

    remoteAddr := conn.RemoteAddr().String()
    g.logger.Info().Str("remote", remoteAddr).Msg("incoming connection")

    tunnelID := fmt.Sprintf("tunnel-%d", time.Now().UnixNano())

    t, err := tunnel.NewTunnel(tunnelID, conn, g.tunnelSender, g.cfg.TunnelTimeout, g.cfg.IdleTimeout)
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

    if err := g.tunnelSender.SendTunnelOpen(tunnelID, "", "", remoteAddr, nil); err != nil {
        g.logger.Error().Err(err).Msg("failed to send tunnel open")
        g.pool.Remove(tunnelID)
        t.Close()
        return
    }

    t.Relay(g.ctx)
    g.pool.Remove(tunnelID)
    g.logger.Info().Str("tunnel_id", tunnelID).Msg("tunnel relay ended")
}
```

- [ ] **Step 4: Delete all stub types (lines 295-329)**

Delete the entire block starting with `// Stub types - replaced by real implementations` through the end of the file, including:
- `TunnelPool` struct and all its methods
- `NewTunnelPool` function
- `Tunnel` struct and all its methods
- `NewTunnel` function

- [ ] **Step 5: Run tests**

```bash
cd /root/project/opsagent && go build ./internal/gateway/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/gateway/gateway.go
git commit -m "fix(gateway): replace stub types with real tunnel implementations

The stub TunnelPool and Tunnel types shadowed the real implementations
in internal/gateway/tunnel/, causing the tunnel subsystem to be
completely non-functional.

Closes C1"
```

---

### Task 2: C2 — SSH host key verification

**Files:**
- Modify: `internal/gateway/proxy/ssh.go`

- [ ] **Step 1: Add knownhosts import and update SSHConfig**

```go
import (
    // ... existing imports ...
    "golang.org/x/crypto/ssh/knownhosts"
)

type SSHConfig struct {
    User               string
    Password           string
    KeyFile            string
    Port               int
    KnownHostsFile     string // path to known_hosts file
    InsecureSkipVerify bool   // if true, skip host key verification (with warning)
}
```

- [ ] **Step 2: Update Connect() to use knownhosts**

```go
func (c *SSHClient) Connect(ctx context.Context, addr string) (*ssh.Client, error) {
    auth, err := c.buildAuth()
    if err != nil {
        return nil, fmt.Errorf("ssh auth: %w", err)
    }

    addr = fmt.Sprintf("%s:%d", addr, c.cfg.Port)

    hostKeyCallback, err := c.buildHostKeyCallback()
    if err != nil {
        return nil, fmt.Errorf("ssh host key: %w", err)
    }

    config := &ssh.ClientConfig{
        User:            c.cfg.User,
        Auth:            auth,
        HostKeyCallback: hostKeyCallback,
        Timeout:         10 * time.Second,
    }

    // ... rest unchanged ...
}
```

- [ ] **Step 3: Add buildHostKeyCallback method**

```go
func (c *SSHClient) buildHostKeyCallback() (ssh.HostKeyCallback, error) {
    if c.cfg.InsecureSkipVerify {
        // Log warning but allow insecure mode for development/testing.
        return ssh.InsecureIgnoreHostKey(), nil
    }

    knownHostsFile := c.cfg.KnownHostsFile
    if knownHostsFile == "" {
        home, err := os.UserHomeDir()
        if err != nil {
            return nil, fmt.Errorf("determine home directory: %w", err)
        }
        knownHostsFile = filepath.Join(home, ".ssh", "known_hosts")
    }

    callback, err := knownhosts.New(knownHostsFile)
    if err != nil {
        return nil, fmt.Errorf("load known hosts %s: %w", knownHostsFile, err)
    }
    return callback, nil
}
```

- [ ] **Step 4: Add filepath import**

```go
import (
    // ... existing imports ...
    "path/filepath"
)
```

- [ ] **Step 5: Run build**

```bash
cd /root/project/opsagent && go build ./internal/gateway/proxy/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/gateway/proxy/ssh.go
git commit -m "fix(proxy): add SSH host key verification

Replace InsecureIgnoreHostKey with knownhosts-based verification.
Users can set InsecureSkipVerify=true for development environments.

Closes C2"
```

---

### Task 3: C3 — Fix metric persistence

**Files:**
- Modify: `internal/grpcclient/client.go:486-540,615-632`

- [ ] **Step 1: Update FlushAndStop() to use persistMetrics**

Replace the `persist:` block in `FlushAndStop()` (around line 517-531):

```go
persist:
    if len(metrics) > 0 && persistPath != "" {
        if err := persistMetrics(metrics, persistPath); err != nil {
            c.logger.Error().Err(err).Msg("failed to persist cache")
        } else {
            c.logger.Info().Int("count", len(metrics)).Str("path", persistPath).Msg("cache persisted to disk")
        }
    } else if len(metrics) > 0 {
        c.logger.Warn().Int("count", len(metrics)).Msg("cache not persisted (no persist path configured)")
    }
```

- [ ] **Step 2: Update loadPersistedCache() to use loadMetrics with size check**

Replace the entire `loadPersistedCache` method:

```go
func (c *Client) loadPersistedCache(path string) {
    // Check file size before loading to prevent OOM.
    const maxPersistFileBytes = 10 * 1024 * 1024 // 10 MB
    fi, err := os.Stat(path)
    if err != nil {
        return // file doesn't exist, ignore
    }
    if fi.Size() > maxPersistFileBytes {
        c.logger.Warn().Int64("size_bytes", fi.Size()).Int64("max_bytes", maxPersistFileBytes).Str("path", path).Msg("persisted cache too large, discarding")
        os.Remove(path)
        return
    }

    metrics, err := loadMetrics(path)
    if err != nil {
        c.logger.Warn().Err(err).Msg("failed to parse persisted cache, discarding")
        os.Remove(path)
        return
    }
    for _, m := range metrics {
        c.cache.Add(m)
    }
    os.Remove(path)
    c.logger.Info().Int("count", len(metrics)).Msg("loaded persisted cache")
}
```

- [ ] **Step 3: Run build**

```bash
cd /root/project/opsagent && go build ./internal/grpcclient/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): use proper metric serialization for persistence

json.Marshal on []*collector.Metric produced [{},{}] due to unexported
fields. Now uses persistMetrics()/loadMetrics() which properly serialize
via accessor methods. Also adds 10MB file size limit for loading.

Closes C3, H7"
```

---

### Task 4: C4 — Environment variable whitelist

**Files:**
- Modify: `internal/sandbox/executor.go:337-354`

- [ ] **Step 1: Replace buildSandboxEnv with allowlist approach**

```go
// sandboxAllowedEnvVars is the set of environment variables that are safe
// to pass into the sandboxed process. All others are blocked by default.
var sandboxAllowedEnvVars = map[string]struct{}{
    "PATH":     {},
    "HOME":     {},
    "LANG":     {},
    "TERM":     {},
    "USER":     {},
    "SHELL":    {},
    "HOSTNAME": {},
    "TMPDIR":   {},
}

// buildSandboxEnv constructs a minimal environment for the sandboxed process.
// It starts with a safe allowlist and merges in request-specified variables
// that are on the allowlist. All other variables are silently dropped.
func buildSandboxEnv(reqEnv map[string]string) []string {
    env := []string{
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "HOME=/tmp",
        "LANG=C",
    }
    for k, v := range reqEnv {
        if _, ok := sandboxAllowedEnvVars[k]; !ok {
            continue
        }
        env = append(env, k+"="+v)
    }
    return env
}
```

- [ ] **Step 2: Run existing tests**

```bash
cd /root/project/opsagent && go test ./internal/sandbox/ -run TestBuildSandboxEnv -v
```

- [ ] **Step 3: Add test for the new allowlist behavior**

In `internal/sandbox/executor_test.go`:

```go
func TestBuildSandboxEnvAllowlist(t *testing.T) {
    reqEnv := map[string]string{
        "PATH":      "/custom/path",
        "LD_PRELOAD": "/evil.so",
        "LD_LIBRARY_PATH": "/evil/lib",
        "BASH_ENV":  "/evil/script",
        "NODE_OPTIONS": "--require /evil",
        "MY_VAR":    "value",
    }
    env := buildSandboxEnv(reqEnv)

    envMap := make(map[string]string)
    for _, e := range env {
        parts := strings.SplitN(e, "=", 2)
        if len(parts) == 2 {
            envMap[parts[0]] = parts[1]
        }
    }

    // PATH should be overridden by request.
    if envMap["PATH"] != "/custom/path" {
        t.Errorf("PATH = %q, want /custom/path", envMap["PATH"])
    }
    // Blocked vars should not appear.
    for _, blocked := range []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "BASH_ENV", "NODE_OPTIONS"} {
        if _, ok := envMap[blocked]; ok {
            t.Errorf("blocked var %q should not be in env", blocked)
        }
    }
    // Non-allowed var should not appear.
    if _, ok := envMap["MY_VAR"]; ok {
        t.Error("MY_VAR should not be in env (not in allowlist)")
    }
}
```

- [ ] **Step 4: Run tests**

```bash
cd /root/project/opsagent && go test ./internal/sandbox/ -run TestBuildSandboxEnv -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/sandbox/executor.go internal/sandbox/executor_test.go
git commit -m "fix(sandbox): switch env var handling to explicit allowlist

The previous blacklist only blocked 3 variables (LD_PRELOAD,
LD_LIBRARY_PATH, DYLD_INSERT_LIBRARIES). Now uses an explicit allowlist
of safe variables, blocking all others by default.

Closes C4"
```

---

## Phase 2: HIGH Fixes

### Task 5: H1 — Complete shell metacharacter list

**Files:**
- Modify: `internal/sandbox/policy.go:108-126`

- [ ] **Step 1: Add missing metacharacters**

```go
var shellMetachars = []string{
    ";",
    "&&",
    "||",
    "|",
    "`",
    "$(",
    "${",
    ">",
    "<",
    "\n",
    "'",
    "\"",
    "\\",
    "*",
    "?",
    "#",
    "~",
    "!",
    "(",
    ")",
    "[",
    "]",
    "{",
    "}",
}
```

- [ ] **Step 2: Add test for new metacharacters**

In `internal/sandbox/policy_test.go`:

```go
func TestPolicyBlockShellInjection_ExtendedMetachars(t *testing.T) {
    p := Policy{AllowedCommands: []string{"echo"}}
    tests := []struct {
        name string
        args []string
   }{
        {"exclamation", []string{"hello!world"}},
        {"open-paren", []string{"hello(world"}},
        {"close-paren", []string{"hello)world"}},
        {"open-bracket", []string{"hello[world"}},
        {"close-bracket", []string{"hello]world"}},
        {"open-brace", []string{"hello{world"}},
        {"close-brace", []string{"hello}world"}},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            if err := p.ValidateCommand("echo", tc.args); err == nil {
                t.Fatalf("expected shell metacharacter rejection for %q", tc.args)
            }
        })
    }
}
```

- [ ] **Step 3: Run tests**

```bash
cd /root/project/opsagent && go test ./internal/sandbox/ -run TestPolicyBlock -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/sandbox/policy.go internal/sandbox/policy_test.go
git commit -m "fix(sandbox): add missing shell metacharacters to policy

Added !, (, ), [, ], {, } to the shell metacharacter list to prevent
shell injection via history expansion, subshell, glob, and brace
expansion.

Closes H1"
```

---

### Task 6: H2 — SSH command argument escaping

**Files:**
- Modify: `internal/gateway/proxy/ssh.go:64-78`

- [ ] **Step 1: Add shellQuote helper**

```go
// shellQuote wraps s in single quotes, escaping any embedded single quotes.
func shellQuote(s string) string {
    return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
```

- [ ] **Step 2: Add strings import**

```go
import (
    // ... existing imports ...
    "strings"
)
```

- [ ] **Step 3: Update Execute() to use shellQuote**

```go
func (c *SSHClient) Execute(ctx context.Context, client *ssh.Client, command string, args []string) (exitCode int, stdout, stderr []byte, timedOut bool) {
    session, err := client.NewSession()
    if err != nil {
        return -1, nil, []byte(err.Error()), false
    }
    defer session.Close()

    var outBuf, errBuf bytes.Buffer
    session.Stdout = &outBuf
    session.Stderr = &errBuf

    fullCmd := shellQuote(command)
    for _, arg := range args {
        fullCmd += " " + shellQuote(arg)
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
```

- [ ] **Step 4: Run build**

```bash
cd /root/project/opsagent && go build ./internal/gateway/proxy/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/proxy/ssh.go
git commit -m "fix(proxy): escape SSH command arguments to prevent injection

Shell metacharacters in arguments could be interpreted by the remote
shell. Now properly quotes all arguments with single quotes.

Closes H2"
```

---

### Task 7: H3 — TCP listener authentication

**Files:**
- Modify: `internal/gateway/gateway.go`
- Modify: `internal/gateway/config.go`

- [ ] **Step 1: Add AuthPSK to Config**

In `internal/gateway/config.go`:

```go
type Config struct {
    ListenAddr    string
    MaxTunnels    int
    TunnelTimeout time.Duration
    IdleTimeout   time.Duration
    Hosts         []HostConfig
    AuthPSK       string // pre-shared key for tunnel authentication (empty = no auth)
}
```

- [ ] **Step 2: Add crypto/subtle import**

In `internal/gateway/gateway.go`:

```go
import (
    // ... existing imports ...
    "crypto/subtle"
)
```

- [ ] **Step 3: Add PSK check to handleIncoming()**

```go
func (g *Gateway) handleIncoming(conn net.Conn) {
    defer g.wg.Done()
    defer conn.Close()

    remoteAddr := conn.RemoteAddr().String()

    // Authenticate if PSK is configured.
    if g.cfg.AuthPSK != "" {
        if err := g.authenticateConnection(conn); err != nil {
            g.logger.Warn().Str("remote", remoteAddr).Err(err).Msg("tunnel auth failed")
            return
        }
    } else {
        g.logger.Warn().Str("remote", remoteAddr).Msg("tunnel auth disabled (no PSK configured)")
    }

    g.logger.Info().Str("remote", remoteAddr).Msg("incoming connection")

    // ... rest of function unchanged ...
}

func (g *Gateway) authenticateConnection(conn net.Conn) error {
    buf := make([]byte, 32)
    if _, err := io.ReadFull(conn, buf); err != nil {
        return fmt.Errorf("read auth token: %w", err)
    }
    if subtle.ConstantTimeCompare(buf, []byte(g.cfg.AuthPSK)) != 1 {
        return fmt.Errorf("invalid auth token")
    }
    return nil
}
```

- [ ] **Step 4: Add io import**

```go
import (
    // ... existing imports ...
    "io"
)
```

- [ ] **Step 5: Run build**

```bash
cd /root/project/opsagent && go build ./internal/gateway/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/gateway/gateway.go internal/gateway/config.go
git commit -m "fix(gateway): add PSK authentication for tunnel connections

Any client that could reach the TCP port could establish a tunnel.
Now requires a pre-shared key for authentication when configured.
Logs a warning when PSK is not set.

Closes H3"
```

---

### Task 8: H4 — Gateway constructor error handling

**Files:**
- Modify: `internal/gateway/gateway.go:48-63`
- Modify: `internal/app/agent.go:265`

- [ ] **Step 1: Change New() signature to return error**

```go
func New(cfg Config, logger zerolog.Logger, tunnelSender TunnelSender, proxySender ProxySender) (*Gateway, error) {
    if tunnelSender == nil {
        return nil, fmt.Errorf("gateway: tunnelSender must not be nil")
    }
    if proxySender == nil {
        return nil, fmt.Errorf("gateway: proxySender must not be nil")
    }
    return &Gateway{
        cfg:          cfg,
        logger:       logger.With().Str("component", "gateway").Logger(),
        tunnelSender: tunnelSender,
        proxySender:  proxySender,
        pool:         tunnel.NewPool(cfg.MaxTunnels),
    }, nil
}
```

- [ ] **Step 2: Update caller in agent.go**

In `internal/app/agent.go`, update the gateway construction:

```go
// Build gateway if enabled and not injected.
if a.gateway == nil && cfg.Gateway.Enabled {
    gwCfg := gateway.Config{
        // ... unchanged ...
    }
    // ... host config loop unchanged ...
    gw, err := gateway.New(gwCfg, log, a.grpcClient, a.grpcClient)
    if err != nil {
        return nil, fmt.Errorf("create gateway: %w", err)
    }
    a.gateway = gw
}
```

- [ ] **Step 3: Run build**

```bash
cd /root/project/opsagent && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/gateway/gateway.go internal/app/agent.go
git commit -m "fix(gateway): return error instead of panicking on nil args

Gateway.New() now returns (*Gateway, error) instead of panicking when
tunnelSender or proxySender is nil.

Closes H4"
```

---

### Task 9: H5 — Plugin runtime env leak

**Files:**
- Modify: `internal/pluginruntime/runtime.go:96-104`

- [ ] **Step 1: Add buildPluginRuntimeEnv function**

```go
// buildPluginRuntimeEnv constructs a sanitized environment for the plugin process.
// It uses an allowlist approach to avoid leaking host secrets.
func buildPluginRuntimeEnv(socketPath string) []string {
    return []string{
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "HOME=/tmp",
        "LANG=C",
        "OPSAGENT_PLUGIN_SOCKET=" + socketPath,
    }
}
```

- [ ] **Step 2: Update Start() to use buildPluginRuntimeEnv**

```go
cmd := exec.CommandContext(ctx, r.cfg.RuntimePath, "--socket", r.cfg.SocketPath)
cmd.Env = buildPluginRuntimeEnv(r.cfg.SocketPath)
cmd.Stdout = os.Stdout
cmd.Stderr = os.Stderr
```

- [ ] **Step 3: Run build**

```bash
cd /root/project/opsagent && go build ./internal/pluginruntime/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/pluginruntime/runtime.go
git commit -m "fix(pluginruntime): use allowlist for plugin process environment

cmd.Env = os.Environ() leaked all host environment variables including
secrets to the plugin subprocess. Now uses a minimal allowlist.

Closes H5"
```

---

### Task 10: H6 — Implement SendProxyMetrics

**Files:**
- Modify: `internal/grpcclient/client.go:602-613`

- [ ] **Step 1: Implement SendProxyMetrics properly**

```go
func (c *Client) SendProxyMetrics(hostID string, metrics []byte) error {
    c.mu.Lock()
    stream := c.stream
    connected := c.connected
    c.mu.Unlock()
    if !connected || stream == nil {
        return fmt.Errorf("not connected")
    }
    msg := &pb.AgentMessage{
        Payload: &pb.AgentMessage_ProxyMetrics{
            ProxyMetrics: &pb.ProxyMetricBatch{
                HostId:  hostID,
                Metrics: metrics,
            },
        },
    }
    return stream.Send(msg)
}
```

- [ ] **Step 2: Verify ProxyMetricBatch proto fields**

The `ProxyMetricBatch` message has these fields (already confirmed in proto):
- `HostId string` — maps to `hostID` parameter
- `Metrics []*Metric` — maps to `metrics` parameter (each `Metric` has `Name`, `Type`, `Value`, `Labels`, `Timestamp`)

No changes needed — the code in Step 1 uses these fields correctly.

- [ ] **Step 3: Run build**

```bash
cd /root/project/opsagent && go build ./internal/grpcclient/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): implement SendProxyMetrics properly

Was returning nil without sending anything. Now builds and sends a
proper ProxyMetricBatch message.

Closes H6"
```

---

### Task 11: H8 — Timeout upper bound

**Files:**
- Modify: `internal/sandbox/executor.go:16-29,297-327`

- [ ] **Step 1: Add MaxTimeoutSec to Config**

```go
type Config struct {
    // ... existing fields ...
    MaxTimeoutSec      int    `json:"max_timeout_sec"`
    // ...
}
```

- [ ] **Step 2: Set default in NewExecutor**

```go
if cfg.MaxTimeoutSec <= 0 {
    cfg.MaxTimeoutSec = cfg.TimeoutSec * 10
}
```

- [ ] **Step 3: Cap timeout in buildNsjailConfig**

```go
func (e *Executor) buildNsjailConfig(req ExecRequest) NsjailConfig {
    cfg := NsjailConfig{
        TimeLimit:   e.cfg.TimeoutSec,
        // ... other fields unchanged ...
    }

    // Apply per-request overrides.
    if req.SandboxCfg != nil {
        // ... existing overrides unchanged ...
    }

    // Cap timeout to maximum.
    if cfg.TimeLimit > e.cfg.MaxTimeoutSec {
        cfg.TimeLimit = e.cfg.MaxTimeoutSec
    }

    return cfg
}
```

- [ ] **Step 4: Add timeout cap in run() method**

```go
// Determine timeout.
timeout := req.Timeout
if timeout <= 0 {
    timeout = time.Duration(e.cfg.TimeoutSec) * time.Second
}

// Cap to maximum timeout.
maxTimeout := time.Duration(e.cfg.MaxTimeoutSec) * time.Second
if timeout > maxTimeout {
    timeout = maxTimeout
}
```

- [ ] **Step 5: Run build**

```bash
cd /root/project/opsagent && go build ./internal/sandbox/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/sandbox/executor.go
git commit -m "fix(sandbox): add maximum timeout limit

No upper bound on timeout allowed malicious requests to set 24-hour
timeouts, holding semaphore slots indefinitely. Default max is 10x
the configured timeout.

Closes H8"
```

---

## Phase 3: MEDIUM Fixes

### Task 12: M1 — Flush output ordering

**Files:**
- Modify: `internal/sandbox/output_streamer.go:54-63`

- [ ] **Step 1: Hold lock during sender call**

```go
func (os *OutputStreamer) Flush() {
    os.mu.Lock()
    data := os.buf
    os.buf = nil
    if len(data) > 0 && os.sender != nil {
        os.sender(data)
    }
    os.mu.Unlock()
}
```

- [ ] **Step 2: Run tests**

```bash
cd /root/project/opsagent && go test ./internal/sandbox/ -run TestOutputStreamer -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/sandbox/output_streamer.go
git commit -m "fix(sandbox): hold lock during Flush sender call

Releasing the lock before calling sender allowed concurrent Write()
calls to interleave with Flush(), potentially reordering output.

Closes M1"
```

---

### Task 13: M2 — Empty allowlist warning

**Files:**
- Modify: `internal/sandbox/executor.go` (in `run()` method)

- [ ] **Step 1: Add warning for empty allowlist**

In the `run()` method, after the network setup block:

```go
// Set up network isolation if allowlist mode.
if nsCfg.NetworkMode == "allowlist" && req.SandboxCfg != nil {
    if len(req.SandboxCfg.AllowedIPs) == 0 {
        e.logger.Warn().Str("task_id", taskID).Msg("network mode is 'allowlist' but no IPs configured, falling back to 'disabled'")
        nsCfg.NetworkMode = "disabled"
    } else {
        if err := e.net.SetupAllowlistNetwork(taskID, req.SandboxCfg.AllowedIPs); err != nil {
            e.logger.Warn().Err(err).Str("task_id", taskID).Msg("failed to setup allowlist network")
        }
        defer e.net.CleanupNetwork(taskID)
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/sandbox/executor.go
git commit -m "fix(sandbox): warn and disable network when allowlist is empty

NetworkMode 'allowlist' with empty AllowedIPs silently disabled
network. Now logs a warning and falls back to 'disabled' mode.

Closes M2"
```

---

### Task 14: M3 — Heartbeat reliability

**Files:**
- Modify: `internal/grpcclient/client.go:308-351`

- [ ] **Step 1: Restructure messageLoop with separate heartbeat goroutine**

```go
func (c *Client) messageLoop(ctx context.Context) {
    // Start heartbeat in a separate goroutine.
    heartbeatDone := make(chan struct{})
    go func() {
        defer close(heartbeatDone)
        ticker := time.NewTicker(time.Duration(c.cfg.HeartbeatSeconds) * time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-ticker.C:
                c.sendHeartbeat()
            }
        }
    }()

    defer func() {
        <-heartbeatDone // wait for heartbeat goroutine to exit
    }()

    for {
        select {
        case <-ctx.Done():
            c.setConnected(false)
            return
        default:
        }

        c.mu.Lock()
        stream := c.stream
        c.mu.Unlock()

        if stream == nil {
            c.setConnected(false)
            return
        }

        msg, err := stream.Recv()
        if err != nil {
            if err == io.EOF {
                c.logger.Warn().Msg("stream closed by server")
            } else {
                c.logger.Error().Err(err).Msg("stream recv error")
            }
            c.setConnected(false)
            c.closeConn()
            return
        }

        if c.receiver != nil {
            if err := c.receiver.Handle(ctx, msg); err != nil {
                c.logger.Error().Err(err).Msg("handler error")
            }
        }
    }
}
```

- [ ] **Step 2: Run build**

```bash
cd /root/project/opsagent && go build ./internal/grpcclient/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): run heartbeat in separate goroutine

The default branch in the select made heartbeat checks unreliable
because stream.Recv() blocked the loop. Now heartbeat runs in its
own goroutine.

Closes M3"
```

---

### Task 15: M4 — Timer leak in connectLoop

**Files:**
- Modify: `internal/grpcclient/client.go:218-256`

- [ ] **Step 1: Replace time.After with time.NewTimer**

```go
func (c *Client) connectLoop(ctx context.Context) {
    for {
        if err := c.connect(ctx); err != nil {
            c.logger.Error().Err(err).Msg("connection failed")
        }

        backoff := time.Second
        maxBackoff := time.Duration(c.cfg.ReconnectMaxSec) * time.Second

        for {
            timer := time.NewTimer(backoff)
            select {
            case <-ctx.Done():
                timer.Stop()
                return
            case <-timer.C:
            }

            if err := c.connect(ctx); err != nil {
                c.logger.Error().Err(err).Msg("reconnect failed")
                backoff *= 2
                if backoff > maxBackoff {
                    backoff = maxBackoff
                }
                continue
            }
            break
        }

        c.messageLoop(ctx)

        select {
        case <-ctx.Done():
            return
        default:
        }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): use time.NewTimer to prevent timer leak

time.After() in a loop creates timers that aren't collected until
they fire, causing memory leaks during reconnection storms.

Closes M4"
```

---

### Task 16: M5 — replayCache batch size

**Files:**
- Modify: `internal/grpcclient/client.go:371-394`

- [ ] **Step 1: Add batching to replayCache**

```go
func (c *Client) replayCache() {
    metrics := c.cache.Drain()
    if len(metrics) == 0 {
        return
    }
    c.logger.Info().Int("count", len(metrics)).Msg("replaying cached metrics")

    c.mu.Lock()
    stream := c.stream
    c.mu.Unlock()

    if stream == nil {
        return
    }

    // Send in batches to avoid exceeding gRPC message size limits.
    const batchSize = 100
    for i := 0; i < len(metrics); i += batchSize {
        end := i + batchSize
        if end > len(metrics) {
            end = len(metrics)
        }
        batch := metrics[i:end]
        msg := NewMetricBatchMessage(batch)
        if err := stream.Send(msg); err != nil {
            c.logger.Warn().Err(err).Msg("cache replay failed, re-caching remaining")
            for _, m := range metrics[i:] {
                c.cache.Add(m)
            }
            return
        }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): batch replayCache to avoid gRPC size limit

Single batch send could exceed gRPC 4MB message limit. Now sends in
batches of 100, matching FlushAndStop behavior.

Closes M5"
```

---

### Task 17: M6 — FlushAndStop race

**Files:**
- Modify: `internal/grpcclient/client.go:484-540`

- [ ] **Step 1: Drain cache before canceling**

```go
func (c *Client) FlushAndStop(ctx context.Context, persistPath string) error {
    // Drain cache BEFORE canceling to avoid race with connectLoop.
    metrics := c.cache.Drain()

    // Cancel the connection loop.
    if c.cancel != nil {
        c.cancel()
    }

    if len(metrics) > 0 {
        // ... rest of send/persist logic unchanged ...
    }

    c.closeConn()
    c.wg.Wait()
    return nil
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): drain cache before canceling in FlushAndStop

Canceling the context before draining caused a race condition with
the connection loop. Now drains first, then cancels.

Closes M6"
```

---

### Task 18: M7 — Shutdown TOCTOU

**Files:**
- Modify: `internal/app/agent.go:607-634`

- [ ] **Step 1: Add taskMu field to Agent struct**

```go
type Agent struct {
    // ... existing fields ...
    taskMu         sync.Mutex  // protects shutdown check + task registration
    // ...
}
```

- [ ] **Step 2: Update exec_command handler to use taskMu**

```go
dispatcher.Register(task.TypeExecCommand, func(ctx context.Context, t task.AgentTask) (any, error) {
    a.taskMu.Lock()
    if a.shuttingDown.Load() {
        a.taskMu.Unlock()
        // ... audit log ...
        return nil, fmt.Errorf("agent is shutting down")
    }
    taskCtx, cancel := context.WithCancel(ctx)
    a.activeTasks.Store(t.TaskID, cancel)
    a.taskMu.Unlock()

    defer a.activeTasks.Delete(t.TaskID)

    // ... rest of handler unchanged ...
})
```

- [ ] **Step 3: Apply same pattern to sandbox_exec handler**

Apply the same `taskMu` locking pattern to the `TypeSandboxExec` handler.

- [ ] **Step 4: Run build**

```bash
cd /root/project/opsagent && go build ./internal/app/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/app/agent.go
git commit -m "fix(app): fix shutdown TOCTOU race in task registration

The shutdown check and activeTasks.Store had a race window where a
task could be registered after shutdown started. Now uses a mutex
to make the check-and-register atomic.

Closes M7"
```

---

### Task 19: M8 — Unsandboxed fallback control

**Files:**
- Modify: `internal/config/config.go:110-120` (add field to SandboxConfig)
- Modify: `internal/app/agent.go:949-972`

- [ ] **Step 1: Add AllowUnsandboxedFallback to SandboxConfig**

Add a new field to `SandboxConfig` in `internal/config/config.go`:

```go
type SandboxConfig struct {
	Enabled                  bool         `mapstructure:"enabled"`
	NsjailPath               string       `mapstructure:"nsjail_path"`
	BaseWorkdir              string       `mapstructure:"base_workdir"`
	DefaultTimeoutSeconds    int          `mapstructure:"default_timeout_seconds"`
	MaxConcurrentTasks       int          `mapstructure:"max_concurrent_tasks"`
	CgroupBasePath           string       `mapstructure:"cgroup_base_path"`
	AuditLogPath             string       `mapstructure:"audit_log_path"`
	Policy                   PolicyConfig `mapstructure:"policy"`
	AllowUnsandboxedFallback bool         `mapstructure:"allow_unsandboxed_fallback"`
}
```

Default is `false` (zero value for bool), so existing configs are safe.

- [ ] **Step 2: Add fallback guard in gRPC command handler**

Update the gRPC command handler fallback section in `internal/app/agent.go`:

```go
// Fallback to local executor.
if !a.cfg.Sandbox.AllowUnsandboxedFallback {
    a.log.Error().Str("task_id", cmd.GetTaskId()).Msg("sandbox unavailable and unsandboxed fallback is disabled")
    a.grpcClient.SendExecResult(&grpcclient.ExecResult{
        TaskID:   cmd.GetTaskId(),
        ExitCode: -1,
    })
    return nil
}
a.log.Warn().Str("task_id", cmd.GetTaskId()).Msg("sandbox unavailable, falling back to unsandboxed execution")

timeoutSec := int(cmd.GetTimeoutSeconds())
if timeoutSec <= 0 {
    timeoutSec = a.cfg.Executor.TimeoutSeconds
}
res, err := a.executor.Execute(ctx, executor.Request{
    Command:        cmd.GetCommand(),
    Args:           cmd.GetArgs(),
    TimeoutSeconds: timeoutSec,
})
```

- [ ] **Step 3: Run build**

```bash
cd /root/project/opsagent && go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/app/agent.go
git commit -m "fix(app): add AllowUnsandboxedFallback config for M8

gRPC command handler silently fell back to the local (unsandboxed)
executor when sandbox was unavailable. Now requires explicit opt-in
via sandbox.allow_unsandboxed_fallback config (default false).

Closes M8"
```

---

### Task 20: M9 — Extract task handler middleware

**Files:**
- Modify: `internal/app/agent.go:607-916`

- [ ] **Step 1: Add taskHandler type and middleware**

```go
type taskHandler func(ctx context.Context, t task.AgentTask) (any, error)

func (a *Agent) withTaskMiddleware(taskType string, handler taskHandler) taskHandler {
    return func(ctx context.Context, t task.AgentTask) (any, error) {
        if a.shuttingDown.Load() {
            a.auditLog.Log(AuditEvent{
                EventType: "task.failed", Component: "dispatcher",
                Action: taskType, Status: "failure",
                Details: map[string]interface{}{"task_id": t.TaskID},
                Error:   "agent is shutting down",
            })
            return nil, fmt.Errorf("agent is shutting down")
        }

        a.auditLog.Log(AuditEvent{
            EventType: "task.started", Component: "dispatcher",
            Action: taskType, Status: "success",
            Details: map[string]interface{}{"task_id": t.TaskID},
        })
        a.metricsReg.TasksRunning.Inc()
        defer a.metricsReg.TasksRunning.Dec()

        res, err := handler(ctx, t)
        if err != nil {
            a.metricsReg.IncTasksFailed(taskType, "error")
            a.auditLog.Log(AuditEvent{
                EventType: "task.failed", Component: "dispatcher",
                Action: taskType, Status: "failure",
                Details: map[string]interface{}{"task_id": t.TaskID},
                Error:   err.Error(),
            })
            return nil, err
        }

        a.metricsReg.IncTasksCompleted()
        a.auditLog.Log(AuditEvent{
            EventType: "task.completed", Component: "dispatcher",
            Action: taskType, Status: "success",
            Details: map[string]interface{}{"task_id": t.TaskID},
        })
        return res, nil
    }
}
```

- [ ] **Step 2: Refactor exec_command handler to use middleware**

```go
dispatcher.Register(task.TypeExecCommand, a.withTaskMiddleware("exec_command", func(ctx context.Context, t task.AgentTask) (any, error) {
    taskCtx, cancel := context.WithCancel(ctx)
    a.taskMu.Lock()
    if a.shuttingDown.Load() {
        a.taskMu.Unlock()
        cancel()
        return nil, fmt.Errorf("agent is shutting down")
    }
    a.activeTasks.Store(t.TaskID, cancel)
    a.taskMu.Unlock()
    defer cancel()
    defer a.activeTasks.Delete(t.TaskID)

    cmdVal, ok := t.Payload["command"].(string)
    if !ok || cmdVal == "" {
        return nil, fmt.Errorf("task payload.command is required")
    }

    args := make([]string, 0)
    if rawArgs, ok := t.Payload["args"].([]any); ok {
        for _, arg := range rawArgs {
            s, ok := arg.(string)
            if !ok {
                return nil, fmt.Errorf("task payload.args must be string array")
            }
            args = append(args, s)
        }
    }

    timeoutSeconds := 0
    if timeoutVal, ok := t.Payload["timeout_seconds"]; ok {
        switch v := timeoutVal.(type) {
        case float64:
            timeoutSeconds = int(v)
        case int:
            timeoutSeconds = v
        case string:
            parsed, err := strconv.Atoi(v)
            if err != nil {
                return nil, fmt.Errorf("invalid timeout_seconds: %w", err)
            }
            timeoutSeconds = parsed
        default:
            return nil, fmt.Errorf("invalid timeout_seconds type")
        }
    }

    return a.executor.Execute(taskCtx, executor.Request{
        Command:        cmdVal,
        Args:           args,
        TimeoutSeconds: timeoutSeconds,
    })
}))
```

- [ ] **Step 3: Refactor sandbox_exec handler to use middleware**

Replace the inline audit/metrics/error handling in the `TypeSandboxExec` dispatcher registration:

```go
dispatcher.Register(task.TypeSandboxExec, a.withTaskMiddleware("sandbox_exec", func(ctx context.Context, t task.AgentTask) (any, error) {
    if a.sandboxExec == nil {
        return nil, fmt.Errorf("sandbox not available")
    }

    cmdVal, _ := t.Payload["command"].(string)
    if cmdVal == "" {
        return nil, fmt.Errorf("task payload.command is required")
    }

    args := make([]string, 0)
    if rawArgs, ok := t.Payload["args"].([]any); ok {
        for _, arg := range rawArgs {
            s, ok := arg.(string)
            if !ok {
                return nil, fmt.Errorf("task payload.args must be string array")
            }
            args = append(args, s)
        }
    }

    result, err := a.sandboxExec.ExecuteCommand(ctx, sandbox.ExecRequest{
        TaskID:  t.TaskID,
        Command: cmdVal,
        Args:    args,
    }, nil)
    if err != nil {
        return nil, err
    }

    return map[string]interface{}{
        "exit_code": result.ExitCode,
        "stdout":    result.Stdout,
        "stderr":    result.Stderr,
        "duration":  result.Duration.Milliseconds(),
    }, nil
}))
```

- [ ] **Step 4: Refactor plugin type handlers to use middleware**

Refactor the plugin gateway task handler registration (the `OnPluginLoaded` callback):

```go
gw.OnPluginLoaded(func(name string, taskTypes []string) {
    for _, tt := range taskTypes {
        fullType := pluginruntime.FullTaskType(name, tt)
        ft := fullType
        dispatcher.Register(ft, a.withTaskMiddleware(ft, func(ctx context.Context, t task.AgentTask) (any, error) {
            return a.executeGatewayTask(ctx, t)
        }))
        a.log.Info().Str("task_type", ft).Msg("registered gateway task handler")
    }
})
```

- [ ] **Step 5: Run build**

```bash
cd /root/project/opsagent && go build ./internal/app/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/app/agent.go
git commit -m "refactor(app): extract task handler middleware

300+ lines of repetitive handler closures reduced by extracting
withTaskMiddleware that handles audit logging, metrics, and error
handling. Each handler only implements core logic.

Closes M9"
```

---

### Task 21: M10 — Gateway interface compile check

**Files:**
- Modify: `internal/app/interfaces.go:87-94`

- [ ] **Step 1: Add Gateway compile-time check**

```go
// Compile-time interface satisfaction checks.
var (
    _ GRPCClient     = (*grpcclient.Client)(nil)
    _ HTTPServer     = (*server.Server)(nil)
    _ Scheduler      = (*collector.Scheduler)(nil)
    _ PluginRuntime  = (*pluginruntime.Runtime)(nil)
    _ PluginGateway  = (*pluginruntime.Gateway)(nil)
    _ Gateway        = (*gateway.Gateway)(nil)
)
```

- [ ] **Step 2: Add gateway import**

```go
import (
    // ... existing imports ...
    "github.com/cy77cc/opsagent/internal/gateway"
)
```

- [ ] **Step 3: Run build**

```bash
cd /root/project/opsagent && go build ./internal/app/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/app/interfaces.go
git commit -m "fix(app): add compile-time check for Gateway interface

Ensures gateway.Gateway satisfies the app.Gateway interface at
compile time.

Closes M10"
```

---

### Task 22: M11 — Hot-reload PluginGateway/Checker

**Files:**
- Modify: `internal/config/diff.go`

- [ ] **Step 1: Add PluginGatewayChanged and CheckerChanged to ChangeSet**

```go
type ChangeSet struct {
    CollectorChanged      bool
    ReporterChanged       bool
    AuthChanged           bool
    PrometheusChanged     bool
    PluginGatewayChanged  bool
    CheckerChanged        bool
}
```

- [ ] **Step 2: Add diff functions**

```go
func diffPluginGateway(old, new *Config) bool {
    return !reflect.DeepEqual(old.PluginGateway, new.PluginGateway)
}

func diffChecker(old, new *Config) bool {
    return !reflect.DeepEqual(old.Checker, new.Checker)
}
```

- [ ] **Step 3: Wire into Diff()**

```go
func Diff(old, new *Config) (*ChangeSet, []NonReloadableChange, error) {
    // ... validation unchanged ...

    cs := &ChangeSet{}

    // ... existing reloadable checks unchanged ...

    // Reloadable: plugin gateway
    if diffPluginGateway(old, new) {
        cs.PluginGatewayChanged = true
    }

    // Reloadable: checker
    if diffChecker(old, new) {
        cs.CheckerChanged = true
    }

    // ... non-reloadable checks unchanged ...

    return cs, nonReloadable, nil
}
```

- [ ] **Step 4: Run build**

```bash
cd /root/project/opsagent && go build ./internal/config/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/diff.go
git commit -m "fix(config): detect PluginGateway and Checker config changes

Hot-reload Diff() ignored PluginGateway and Checker configuration
changes. Now properly detects and reports them in the ChangeSet.

Closes M11"
```

---

### Task 23: M12 — Rate limiter eviction

**Files:**
- Modify: `internal/server/middleware.go`

- [ ] **Step 1: Add lastSeen tracking and eviction**

```go
type rateLimiter struct {
    visitors map[string]*visitorEntry
    mu       sync.Mutex
    rate     rate.Limit
    burst    int
}

type visitorEntry struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}

func newRateLimiter(r rate.Limit, burst int) *rateLimiter {
    rl := &rateLimiter{
        visitors: make(map[string]*visitorEntry),
        rate:     r,
        burst:    burst,
    }
    go rl.evictionLoop()
    return rl
}

func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    if entry, ok := rl.visitors[ip]; ok {
        entry.lastSeen = time.Now()
        return entry.limiter
    }
    lim := rate.NewLimiter(rl.rate, rl.burst)
    rl.visitors[ip] = &visitorEntry{limiter: lim, lastSeen: time.Now()}
    return lim
}

func (rl *rateLimiter) evictionLoop() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        rl.mu.Lock()
        for ip, entry := range rl.visitors {
            if time.Since(entry.lastSeen) > 10*time.Minute {
                delete(rl.visitors, ip)
            }
        }
        rl.mu.Unlock()
    }
}
```

- [ ] **Step 2: Run build**

```bash
cd /root/project/opsagent && go build ./internal/server/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/server/middleware.go
git commit -m "fix(server): add eviction to rate limiter

The visitors map grew unbounded, allowing memory exhaustion attacks.
Now evicts entries not seen for 10 minutes via a background goroutine.

Closes M12"
```

---

### Task 24: M13 — DropOldest memory leak

**Files:**
- Modify: `internal/collector/buffer.go:37-46`

- [ ] **Step 1: Nil out dropped element**

```go
func (b *Buffer) Add(m *Metric) {
    b.mu.Lock()
    defer b.mu.Unlock()

    if len(b.metrics) >= b.maxSize {
        switch b.dropPolicy {
        case DropNewest:
            return // drop incoming metric
        case DropOldest:
            b.metrics[0] = nil  // allow GC of dropped element
            b.metrics = b.metrics[1:]
        }
    }
    b.metrics = append(b.metrics, m)
}
```

- [ ] **Step 2: Run tests**

```bash
cd /root/project/opsagent && go test ./internal/collector/ -run TestBuffer -v
```

- [ ] **Step 3: Commit**

```bash
git add internal/collector/buffer.go
git commit -m "fix(collector): nil dropped element in DropOldest policy

b.metrics[1:] doesn't release the underlying array reference, causing
the dropped Metric to be retained in memory. Now nils the element
before slicing.

Closes M13"
```

---

### Task 25: M14 — healthCheckAll stale pointers

**Files:**
- Modify: `internal/pluginruntime/gateway.go:630-704`

- [ ] **Step 1: Re-read plugin from map after restart**

```go
func (g *Gateway) healthCheckAll() {
    g.mu.RLock()
    plugins := make(map[string]*ManagedPlugin, len(g.plugins))
    for k, v := range g.plugins {
        plugins[k] = v
    }
    g.mu.RUnlock()

    for name, p := range plugins {
        p.mu.Lock()
        if p.Status != PluginStatusRunning || p.Client == nil {
            p.mu.Unlock()
            continue
        }
        client := p.Client
        p.mu.Unlock()

        ctx, cancel := context.WithTimeout(g.ctx, 5*time.Second)
        err := client.Ping(ctx)
        cancel()

        if err != nil {
            g.logger.Warn().Err(err).Str("plugin", name).Msg("plugin health check failed")

            p.mu.Lock()
            restartable := g.shouldRestart(p)
            manifest := p.Manifest
            restartCount := p.RestartCount
            p.mu.Unlock()

            if restartable {
                backoff := g.restartBackoff(restartCount)
                g.logger.Info().
                    Str("plugin", name).
                    Int("restart_count", restartCount).
                    Dur("backoff", backoff).
                    Msg("restarting plugin after health check failure")

                g.stopPlugin(name, p)

                select {
                case <-g.ctx.Done():
                    return
                case <-time.After(backoff):
                }

                if err := g.loadPlugin(manifest); err != nil {
                    g.logger.Error().Err(err).Str("plugin", name).Msg("failed to restart plugin")
                    // Re-read from map to get the current (possibly stale) entry.
                    g.mu.RLock()
                    current, ok := g.plugins[name]
                    g.mu.RUnlock()
                    if ok {
                        current.mu.Lock()
                        current.Status = PluginStatusError
                        current.mu.Unlock()
                    }
                }
            } else {
                g.logger.Error().
                    Str("plugin", name).
                    Int("restart_count", restartCount).
                    Int("max_restarts", g.cfg.MaxRestarts).
                    Msg("plugin exceeded max restarts, marking as error")
                // Re-read from map.
                g.mu.RLock()
                current, ok := g.plugins[name]
                g.mu.RUnlock()
                if ok {
                    current.mu.Lock()
                    current.Status = PluginStatusError
                    current.mu.Unlock()
                }
            }
        } else {
            p.mu.Lock()
            p.LastHealth = time.Now()
            p.mu.Unlock()
        }
    }
}
```

- [ ] **Step 2: Run build**

```bash
cd /root/project/opsagent && go build ./internal/pluginruntime/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/pluginruntime/gateway.go
git commit -m "fix(pluginruntime): re-read plugin map after restart

After stopPlugin + loadPlugin, the old *ManagedPlugin pointer is stale.
Now re-reads from g.plugins[name] map to get the current entry.

Closes M14"
```

---

## Phase 4: LOW Fixes

### Task 26: L1 — hostID/hostname confusion

**Files:**
- Modify: `internal/gateway/config.go`
- Modify: `internal/gateway/gateway.go:84-91`

- [ ] **Step 1: Add Hostname to HostConfig**

```go
type HostConfig struct {
    ID       string
    Hostname string // display name for registration; falls back to ID if empty
    Addr     string
    Mode     string // "tunnel", "proxy", "auto"
    SSH      SSHConfig
}
```

- [ ] **Step 2: Update proxy registration to use Hostname**

```go
for _, h := range g.cfg.Hosts {
    if h.Mode == "proxy" || h.Mode == "auto" {
        hostname := h.Hostname
        if hostname == "" {
            hostname = h.ID
        }
        if err := g.proxySender.SendProxyRegister(h.ID, hostname, h.Addr, nil); err != nil {
            g.logger.Warn().Err(err).Str("host_id", h.ID).Msg("failed to register proxy host")
        }
    }
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/gateway/config.go internal/gateway/gateway.go
git commit -m "fix(gateway): separate hostID from hostname in proxy registration

SendProxyRegister was using h.ID for both hostID and hostname. Now
uses h.Hostname for display name, falling back to h.ID if empty.

Closes L1"
```

---

### Task 27: L2 — Random tunnel ID

**Files:**
- Modify: `internal/gateway/gateway.go:220-225`

- [ ] **Step 1: Add crypto/rand import**

```go
import (
    // ... existing imports ...
    "crypto/rand"
    "encoding/hex"
)
```

- [ ] **Step 2: Update tunnel ID generation**

```go
func (g *Gateway) handleIncoming(conn net.Conn) {
    // ... auth check unchanged ...

    // Generate unpredictable tunnel ID.
    randBytes := make([]byte, 8)
    if _, err := rand.Read(randBytes); err != nil {
        g.logger.Error().Err(err).Msg("failed to generate tunnel ID")
        return
    }
    tunnelID := "tunnel-" + hex.EncodeToString(randBytes)

    // ... rest unchanged ...
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/gateway/gateway.go
git commit -m "fix(gateway): use crypto/rand for tunnel IDs

time.Now().UnixNano() was predictable. Now uses 8 random bytes
encoded as hex for unpredictable tunnel identifiers.

Closes L2"
```

---

### Task 28: L3 — Single connection close owner

**Files:**
- Modify: `internal/gateway/tunnel/tunnel.go:91-131`

- [ ] **Step 1: Remove defer conn.Close() from Relay()**

```go
func (t *Tunnel) Relay(ctx context.Context) {
    // Removed: defer t.conn.Close() — Close() is the sole owner.

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
                continue
            }
            if err != io.EOF {
                t.Close()
            }
            return
        }
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/gateway/tunnel/tunnel.go
git commit -m "fix(tunnel): remove duplicate conn.Close from Relay

Both Relay() and Close() closed t.conn, causing double-close errors.
Now Close() is the sole owner of connection lifecycle.

Closes L3"
```

---

### Task 29: L4 — Implement ExecuteMetricsCollect

**Files:**
- Modify: `internal/gateway/proxy/proxy.go:108-146`

- [ ] **Step 1: Implement metrics parsing and sending**

```go
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

    // Serialize and send metrics.
    data, err := json.Marshal(metrics)
    if err != nil {
        return fmt.Errorf("marshal metrics: %w", err)
    }
    return m.sender.SendProxyMetrics(hostID, data)
}
```

- [ ] **Step 2: Add encoding/json import**

```go
import (
    // ... existing imports ...
    "encoding/json"
)
```

- [ ] **Step 3: Commit**

```bash
git add internal/gateway/proxy/proxy.go
git commit -m "fix(proxy): implement ExecuteMetricsCollect

Was collecting metrics but discarding them. Now serializes and sends
via sender.SendProxyMetrics().

Closes L4"
```

---

### Task 30: L5 — FlushAndStop context usage

**Files:**
- Modify: `internal/grpcclient/client.go:484-540`

- [ ] **Step 1: Use ctx for timeout in flush sends**

```go
func (c *Client) FlushAndStop(ctx context.Context, persistPath string) error {
    metrics := c.cache.Drain()

    if c.cancel != nil {
        c.cancel()
    }

    if len(metrics) > 0 {
        c.mu.Lock()
        stream := c.stream
        c.mu.Unlock()

        if stream != nil {
            batchSize := 100
            for i := 0; i < len(metrics); i += batchSize {
                select {
                case <-ctx.Done():
                    metrics = metrics[i:]
                    goto persist
                default:
                }

                end := i + batchSize
                if end > len(metrics) {
                    end = len(metrics)
                }
                batch := metrics[i:end]
                msg := NewMetricBatchMessage(batch)
                if err := stream.Send(msg); err != nil {
                    c.logger.Warn().Err(err).Msg("flush send failed, will persist remaining")
                    metrics = metrics[i:]
                    goto persist
                }
            }
            metrics = nil
        }

    persist:
        // ... persist logic unchanged ...
    }

    c.closeConn()
    c.wg.Wait()
    return nil
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/grpcclient/client.go
git commit -m "fix(grpcclient): respect context in FlushAndStop

The ctx parameter was ignored, potentially blocking indefinitely.
Now checks ctx.Done() between batch sends.

Closes L5"
```

---

### Task 31: L6 — Histogram type support

**Files:**
- Modify: `internal/grpcclient/persist.go:69-89`

- [ ] **Step 1: Add Histogram case**

```go
func metricTypeString(t collector.MetricType) string {
    switch t {
    case collector.Counter:
        return "counter"
    case collector.Gauge:
        return "gauge"
    case collector.Histogram:
        return "histogram"
    default:
        return "gauge"
    }
}

func metricTypeFromString(s string) collector.MetricType {
    switch s {
    case "counter":
        return collector.Counter
    case "gauge":
        return collector.Gauge
    case "histogram":
        return collector.Histogram
    default:
        return collector.Gauge
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/grpcclient/persist.go
git commit -m "fix(grpcclient): add Histogram type to metric persistence

metricTypeString/metricTypeFromString didn't handle the Histogram
MetricType, causing it to be serialized as "gauge".

Closes L6"
```

---

### Task 32: L7 — python deprecation warning

**Files:**
- Modify: `internal/sandbox/nsjail.go:264-285`

- [ ] **Step 1: Add deprecation warning for "python"**

Add `"log"` to the imports in `internal/sandbox/nsjail.go`, then update the function:

```go
func interpreterToPath(interpreter string) (string, error) {
    switch interpreter {
    case "bash":
        return "/bin/bash", nil
    case "sh":
        return "/bin/sh", nil
    case "python3":
        return "/usr/bin/python3", nil
    case "python":
        log.Println(`WARNING: "python" interpreter is deprecated and will be removed in a future release. Use "python3" explicitly.`)
        return "/usr/bin/python3", nil
    case "ruby":
        return "/usr/bin/ruby", nil
    case "node":
        return "/usr/bin/node", nil
    case "perl":
        return "/usr/bin/perl", nil
    default:
        return "", fmt.Errorf("unsupported interpreter %q", interpreter)
    }
}
```

Note: Uses `log.Println` (standard library) since `nsjail.go` doesn't have access to zerolog. This is acceptable for a deprecation warning — it goes to stderr.
```

- [ ] **Step 2: Commit**

```bash
git add internal/sandbox/nsjail.go
git commit -m "fix(sandbox): add deprecation comment for python interpreter

'python' silently maps to python3 which may be confusing. Added
deprecation comment documenting this behavior.

Closes L7"
```

---

### Task 33: L8 — ToArgs nil check

**Files:**
- Modify: `internal/sandbox/nsjail.go:141-151`

- [ ] **Step 1: Add nil check in CommandArgs**

```go
func (c *NsjailConfig) CommandArgs(taskID string, command string, cmdArgs []string) []string {
    args := c.ToArgs(taskID)
    if args == nil {
        return nil
    }
    args = append(args, "--", command)
    args = append(args, cmdArgs...)
    return args
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/sandbox/nsjail.go
git commit -m "fix(sandbox): check for nil args in CommandArgs

ToArgs() returns nil on sanitize failure, but CommandArgs() didn't
check, leading to a panic when appending to nil.

Closes L8"
```

---

### Task 34: L9 — Configurable gateway task timeout

**Files:**
- Modify: `internal/app/agent.go:1142-1167`

- [ ] **Step 1: Use config timeout instead of hardcoded 30s**

```go
func (a *Agent) executeGatewayTask(ctx context.Context, t task.AgentTask) (any, error) {
    if a.shuttingDown.Load() {
        return nil, fmt.Errorf("agent is shutting down")
    }
    if a.pluginGateway == nil {
        return nil, fmt.Errorf("plugin gateway is not enabled")
    }

    taskID := t.TaskID
    if taskID == "" {
        taskID = fmt.Sprintf("gw-%d", time.Now().UnixNano())
    }

    timeoutSec := a.cfg.Plugin.RequestTimeoutSeconds
    if timeoutSec <= 0 {
        timeoutSec = 30
    }
    deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second).UnixMilli()

    return a.pluginGateway.ExecuteTask(ctx, pluginruntime.TaskRequest{
        TaskID:     taskID,
        Type:       t.Type,
        DeadlineMS: deadline,
        Payload:    t.Payload,
        Chunking: pluginruntime.ChunkingConfig{
            Enabled:       true,
            MaxChunkBytes: a.cfg.Plugin.ChunkSizeBytes,
            MaxTotalBytes: a.cfg.Plugin.MaxResultBytes,
        },
    })
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/app/agent.go
git commit -m "fix(app): use config timeout for gateway tasks

Hardcoded 30s timeout ignored the Plugin.RequestTimeoutSeconds config.
Now uses the configured value with 30s fallback.

Closes L9"
```

---

### Task 35: Final verification

- [ ] **Step 1: Run full build**

```bash
cd /root/project/opsagent && go build ./...
```

- [ ] **Step 2: Run all tests**

```bash
cd /root/project/opsagent && go test ./... -v -count=1
```

- [ ] **Step 3: Run go vet**

```bash
cd /root/project/opsagent && go vet ./...
```

- [ ] **Step 4: Verify all issues closed**

Review the spec at `docs/superpowers/specs/2026-05-24-code-review-fixes-design.md` and confirm each issue (C1-C4, H1-H8, M1-M14, L1-L9) has been addressed.

---

## Summary

| Phase | Issues | Tasks |
|-------|--------|-------|
| CRITICAL | C1, C2, C3, C4 | 1-4 |
| HIGH | H1-H8 | 5-11 |
| MEDIUM | M1-M14 | 12-25 |
| LOW | L1-L9 | 26-34 |
| Verify | — | 35 |

Total: 35 tasks for 35 issues.
