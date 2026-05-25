# OpsAgent Security Hardening Manual

This document comprehensively documents OpsAgent's security architecture and hardening measures. OpsAgent is a host-side metric collection and sandbox execution agent that employs a defense-in-depth strategy, implementing security controls at the network layer, process layer, filesystem layer, and API layer.

---

## 1. Security Architecture Overview

OpsAgent employs a defense-in-depth strategy, implementing security controls at multiple layers:

| Layer | Security Measures |
|------|----------|
| **Network Layer** | TLS 1.2+ enforcement, localhost binding, network isolation (nsjail net namespace) |
| **Process Layer** | nsjail namespace isolation, seccomp syscall whitelist, cgroup v2 resource limits |
| **Filesystem Layer** | Path traversal protection, file permission control (0600), unpredictable temporary paths |
| **API Layer** | Bearer Token authentication, rate limiting, input validation, security response headers |

---

## 2. Sandbox Security

The sandbox subsystem is implemented based on nsjail, providing multi-layer isolation protection. The sandbox is disabled by default and must be explicitly enabled in the configuration.

### 2.1 nsjail Isolation

nsjail provides the following namespace isolation:

- **PID namespace**: Processes inside the sandbox cannot see or signal host processes
- **NET namespace**: Network access disabled by default (`NetworkMode: "disabled"`), with optional allowlist mode
- **MNT namespace**: Read-only bind mounts for `/usr`, `/lib`, `/lib64`, `/bin`; `/etc` uses tmpfs replacement

**UID/GID Mapping**: Runs as nobody (65534) inside the sandbox:

```go
// internal/sandbox/nsjail.go
args = append(args,
    "--uid_mapping=0:65534:1",
    "--gid_mapping=0:65534:1",
)
```

**seccomp Syscall Whitelist** (dynamic policy: base + network):

```go
// internal/sandbox/nsjail.go
const baseSyscalls = `ALLOW {
    read, write, open, close, mmap, munmap, mprotect, brk,
    access, stat, fstat, lstat, ioctl, pread64, pwrite64,
    readv, writev, pipe, dup, dup2, nanosleep, getpid,
    execve, exit, wait4, kill, uname,
    fcntl, flock, fsync, fdatasync, truncate, ftruncate,
    getdents, getcwd, chdir, rename, mkdir, rmdir, link,
    unlink, readlink, chmod, chown, umask, gettimeofday,
    getuid, getgid, geteuid, getegid, getppid, getpgrp,
    set_tid_address, futex, epoll_create, epoll_ctl,
    epoll_wait, clock_gettime, exit_group, set_robust_list,
    openat, mkdirat, newfstatat, unlinkat, renameat,
    readlinkat, faccessat, epoll_create1, pipe2, dup3,
    prlimit64, getrandom, rseq, sigaltstack, rt_sigaction,
    rt_sigprocmask, madvise, getpeername, getsockname,
    timerfd_create, timerfd_settime, timerfd_gettime
}`
```

**Fork Bomb Protection**: `clone`, `fork`, `vfork` are intentionally excluded from the whitelist to prevent fork bomb attacks. Test cases verify this:

```go
// internal/sandbox/sanitize_test.go
if strings.Contains(policy, "clone") || strings.Contains(policy, "fork") || strings.Contains(policy, "vfork") {
    t.Errorf("seccomp policy should not allow clone/fork/vfork")
}
```

**Network syscalls** are only appended when network mode is `allowlist`:

```go
// internal/sandbox/nsjail.go
const networkSyscalls = `,
    socket, connect, bind, listen, accept, accept4,
    sendto, recvfrom, sendmsg, recvmsg, shutdown,
    setsockopt, getsockopt, socketpair, eventfd2`
```

### 2.2 cgroup v2 Resource Limits

Each sandbox task creates an independent cgroup with the following resource limits:

| Resource | Default Limit | Configuration Field |
|------|----------|----------|
| Memory limit | 128 MB | `memory.max` |
| CPU quota | 50% | `cpu.max` (format: `$MAX $PERIOD`) |
| Process limit | 32 | `pids.max` |

Resource limits are written via the cgroup v2 file interface:

```go
// internal/sandbox/stats.go
func SetCgroupLimits(cgroupPath string, memoryMB, cpuPercent, maxPIDs int) error {
    if memoryMB > 0 {
        memBytes := memoryMB * 1024 * 1024
        os.WriteFile(filepath.Join(cgroupPath, "memory.max"), []byte(strconv.Itoa(memBytes)), 0o644)
    }
    if cpuPercent > 0 {
        period := 100000
        max := cpuPercent * period / 100
        val := fmt.Sprintf("%d %d", max, period)
        os.WriteFile(filepath.Join(cgroupPath, "cpu.max"), []byte(val), 0o644)
    }
    if maxPIDs > 0 {
        os.WriteFile(filepath.Join(cgroupPath, "pids.max"), []byte(strconv.Itoa(maxPIDs)), 0o644)
    }
    return nil
}
```

**Resource Boundary Validation**: Request-level overrides have hard limits to prevent resource abuse:

```go
// internal/sandbox/executor.go
if req.SandboxCfg.MemoryMB > 0 {
    cfg.MemoryMB = min(req.SandboxCfg.MemoryMB, 1024)   // Max 1GB
}
if req.SandboxCfg.CPUPercent > 0 {
    cfg.CPUPercent = min(req.SandboxCfg.CPUPercent, 100) // Max 100%
}
if req.SandboxCfg.MaxPIDs > 0 {
    cfg.MaxPIDs = min(req.SandboxCfg.MaxPIDs, 256)       // Max 256 processes
}
```

After task execution completes, all processes in the cgroup are terminated and the cgroup directory is cleaned up:

```go
// internal/sandbox/executor.go
defer func() {
    KillCgroupProcesses(cgroupPath)
    RemoveCgroup(cgroupPath)
}()
```

### 2.3 Security Policy Engine

The policy engine is located at `internal/sandbox/policy.go` and performs multi-layer validation on commands and scripts.

**Command Validation Flow** (`ValidateCommand`):

1. **Shell Metacharacter Detection**: Command names must not contain the following dangerous characters:
   ```go
   // internal/sandbox/policy.go
   var shellMetachars = []string{
       ";", "&&", "||", "|", "`", "$(", "${", ">", "<",
       "\n", "'", "\"", "\\", "*", "?", "#", "~",
   }
   ```
2. **Blacklist Check**: Commands in `blocked_commands` are always rejected (highest priority)
3. **Whitelist Check**: If `allowed_commands` is non-empty, the command must be in the whitelist
4. **Argument Metacharacter Check**: All command arguments are also checked for metacharacters
5. **Sudo Interception**: Rejects sudo unless `allow_sudo: true` is explicitly set

**Script Validation Flow** (`ValidateScript`):

1. **Interpreter Whitelist**: Script interpreter must be in `allowed_interpreters`
2. **Script Size Limit**: Default 64KB (`script_max_bytes: 65536`), max 1MB
3. **Keyword Blacklist** (case-insensitive): Script content must not contain keywords in `blocked_keywords`

**Unknown Interpreter Rejection**: Interpreter names are mapped to absolute paths; unknown names cause an error:

```go
// internal/sandbox/nsjail.go
func interpreterToPath(interpreter string) (string, error) {
    switch interpreter {
    case "bash":  return "/bin/bash", nil
    case "sh":    return "/bin/sh", nil
    case "python3": return "/usr/bin/python3", nil
    // ...
    default:
        return "", fmt.Errorf("unsupported interpreter %q", interpreter)
    }
}
```

### 2.4 Input Validation

**TaskID Path Traversal Protection**: All entry points sanitize TaskID to prevent path traversal attacks:

```go
// internal/sandbox/nsjail.go
func sanitizeTaskID(taskID string) (string, error) {
    if taskID == "" {
        return "", fmt.Errorf("task ID is required")
    }
    cleaned := filepath.Clean(taskID)
    if cleaned == "." || cleaned == ".." || strings.ContainsAny(cleaned, `/\`) {
        return "", fmt.Errorf("invalid task ID: %q contains path traversal", taskID)
    }
    if strings.ContainsRune(cleaned, '\x00') {
        return "", fmt.Errorf("invalid task ID: contains null byte")
    }
    return cleaned, nil
}
```

This function is applied to:
- nsjail argument construction (`ToArgs`, `CommandArgs`, `ScriptArgs`)
- Script file writing (`WriteScriptFile`)
- Configuration file writing (`WriteConfigFile`)
- cgroup creation (`CreateCgroup`)
- Network management (`SetupAllowlistNetwork`, `CleanupNetwork`)

**IP Address Validation**: Validates IP addresses before creating iptables rules to prevent command injection:

```go
// internal/sandbox/network.go
func validateIPList(ips []string) error {
    for _, ip := range ips {
        ip = strings.TrimSpace(ip)
        if ip == "" {
            return fmt.Errorf("empty IP address")
        }
        if strings.Contains(ip, "/") {
            if _, _, err := net.ParseCIDR(ip); err != nil {
                return fmt.Errorf("invalid CIDR %q: %w", ip, err)
            }
        } else {
            if net.ParseIP(ip) == nil {
                return fmt.Errorf("invalid IP address %q", ip)
            }
        }
    }
    return nil
}
```

**Command Name Metacharacter Check**: Even if a command is in the whitelist, it still checks whether it contains shell metacharacters:

```go
// internal/sandbox/policy.go
func (p *Policy) ValidateCommand(command string, args []string) error {
    cmdName := strings.TrimSpace(command)
    if containsShellMetacharacters(cmdName) {
        return fmt.Errorf("command name contains shell metacharacters: %q", cmdName)
    }
    // ... subsequent checks
}
```

### 2.5 Environment Variable Security

Sandbox and plugin processes use minimal environment variables to block library injection attacks:

```go
// internal/sandbox/executor.go
func buildSandboxEnv(reqEnv map[string]string) []string {
    env := []string{
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "HOME=/tmp",
        "LANG=C",
    }
    for k, v := range reqEnv {
        if k == "LD_PRELOAD" || k == "LD_LIBRARY_PATH" || k == "DYLD_INSERT_LIBRARIES" {
            continue  // Block dynamic library injection
        }
        env = append(env, k+"="+v)
    }
    return env
}
```

**Blocked Environment Variables**:
- `LD_PRELOAD`: Linux dynamic library preload injection
- `LD_LIBRARY_PATH`: Dynamic library search path hijacking
- `DYLD_INSERT_LIBRARIES`: macOS dynamic library injection

Plugin environment variables also apply the same filtering rules:

```go
// internal/pluginruntime/gateway.go
func buildPluginEnv(socketPath string, manifestEnv map[string]string) []string {
    env := []string{
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "OPSAGENT_PLUGIN_SOCKET=" + socketPath,
    }
    for k, v := range manifestEnv {
        if k == "LD_PRELOAD" || k == "LD_LIBRARY_PATH" || k == "DYLD_INSERT_LIBRARIES" {
            continue
        }
        env = append(env, k+"="+v)
    }
    return env
}
```

---

## 3. API Security

### 3.1 Authentication

API authentication is based on Bearer Token, using constant-time comparison to prevent timing attacks:

```go
// internal/server/middleware.go
func (s *Server) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !s.requiresAuth(r.URL.Path) {
            next.ServeHTTP(w, r)
            return
        }
        auth := strings.TrimSpace(r.Header.Get("Authorization"))
        expected := "Bearer " + s.options.Auth.BearerToken
        if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
            writeJSON(w, http.StatusUnauthorized, apiResponse{Success: false, Error: "unauthorized"})
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

**Authentication Enabled by Default**:

```go
// internal/config/config.go
v.SetDefault("auth.enabled", true)
```

**Minimum Token Length Requirement** (32 characters):

```go
// internal/config/config.go
if c.Auth.Enabled {
    token := strings.TrimSpace(c.Auth.BearerToken)
    if token == "" {
        return fmt.Errorf("auth.bearer_token is required when auth.enabled=true")
    }
    if len(token) < 32 {
        return fmt.Errorf("auth.bearer_token must be at least 32 characters when auth.enabled=true")
    }
}
```

**Hot Reload Prohibits Disabling Authentication**:

```go
// internal/server/reload.go
func (r *AuthReloader) Apply(newCfg *config.Config) error {
    if !newCfg.Auth.Enabled {
        return fmt.Errorf("auth cannot be disabled via hot-reload (restart required)")
    }
    if newCfg.Auth.BearerToken == "" {
        return fmt.Errorf("bearer_token cannot be empty")
    }
    if len(newCfg.Auth.BearerToken) < 32 {
        return fmt.Errorf("bearer_token must be at least 32 characters")
    }
    // ... apply update
}
```

**Version Information Hiding**: Unauthenticated requests to the health check endpoint do not expose version information:

```go
// internal/server/handlers.go
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
    // ...
    data := map[string]any{
        "status":     overallStatus,
        "subsystems": subsystems,
    }
    // Only expose version info when auth is enabled and request is authenticated
    if s.options.Auth.Enabled {
        auth := strings.TrimSpace(r.Header.Get("Authorization"))
        expected := "Bearer " + s.options.Auth.BearerToken
        if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) == 1 {
            data["version"] = s.version
            data["git_commit"] = s.gitCommit
            data["uptime_seconds"] = int(time.Since(s.startedAt).Seconds())
        }
    }
    // ...
}
```

Paths requiring authentication:
- All endpoints with `/api/v1/` prefix
- Prometheus metrics endpoint (when `protect_with_auth: true`)

```go
// internal/server/middleware.go
func (s *Server) requiresAuth(path string) bool {
    if !s.options.Auth.Enabled {
        return false
    }
    if strings.HasPrefix(path, "/api/v1/") {
        return true
    }
    if s.options.Prometheus.Enabled && s.options.Prometheus.ProtectWithAuth && path == s.options.Prometheus.Path {
        return true
    }
    return false
}
```

### 3.2 Rate Limiting

IP-based rate limiting, with independent rate limiting per visitor:

```go
// internal/server/middleware.go
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
    rl := newRateLimiter(10, 20) // 10 requests/second, burst capacity 20
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := r.RemoteAddr
        if !rl.getLimiter(ip).Allow() {
            writeJSON(w, http.StatusTooManyRequests, apiResponse{Success: false, Error: "rate limit exceeded"})
            return
        }
        next.ServeHTTP(w, r)
    })
}
```

Each IP maintains an independent token bucket rate limiter:

```go
// internal/server/middleware.go
type rateLimiter struct {
    visitors map[string]*rate.Limiter
    mu       sync.Mutex
    rate     rate.Limit
    burst    int
}
```

### 3.3 Input Validation

**Request Body Size Limit**: exec and task endpoints limit request body to 1MB:

```go
// internal/server/handlers.go
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
    // ...
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
    // ...
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
    // ...
    r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
    // ...
}
```

**HTTP Method Restriction**: All endpoints strictly restrict HTTP methods:

```go
// internal/server/handlers.go
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false, Error: "method not allowed"})
        return
    }
    // ...
}
```

**Timeout Limit**: Task timeout is restricted to 300 seconds:

```go
// internal/server/handlers.go
const maxTimeoutSeconds = 300

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
    // ...
    timeoutSeconds := 15
    if timeoutVal, ok := req.Payload["timeout_seconds"]; ok {
        if seconds, ok := parseTimeoutSeconds(timeoutVal); ok && seconds > 0 {
            timeoutSeconds = min(seconds, maxTimeoutSeconds)
        }
    }
    // ...
}
```

### 3.4 Output Security

**Error Message Sanitization**: API responses do not leak internal implementation details:

```go
// internal/server/handlers.go
func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
    // ...
    res, err := s.executor.Execute(r.Context(), req)
    if err != nil {
        s.logger.Error().Err(err).Msg("exec request failed")
        writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Error: "command execution failed"})
        return  // Do not return err.Error() to avoid leaking internal details
    }
    // ...
}
```

**Prometheus Label Value Escaping**: Prevents label injection:

```go
// internal/collector/outputs/prometheus/prometheus.go
func escapeLabelValue(s string) string {
    s = strings.ReplaceAll(s, `\`, `\\`)
    s = strings.ReplaceAll(s, `"`, `\"`)
    s = strings.ReplaceAll(s, "\n", `\n`)
    return s
}
```

### 3.5 HTTP Security Headers

All responses automatically include security headers:

```go
// internal/server/middleware.go
func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Cache-Control", "no-store")
        next.ServeHTTP(w, r)
    })
}
```

Middleware chain order (from outer to inner):

```
recover -> securityHeaders -> rateLimit -> logging -> auth -> handler
```

---

## 4. Communication Security

### 4.1 gRPC

gRPC client enforces TLS 1.2+, refusing insecure connections:

```go
// internal/grpcclient/client.go
func (c *Client) buildTLSCredentials() (credentials.TransportCredentials, error) {
    if c.cfg.CertPath == "" && c.cfg.KeyPath == "" && c.cfg.CAPath == "" {
        return nil, fmt.Errorf("no TLS certificates configured; refusing insecure connection (set grpc.mtls.cert_file, key_file, and ca_file)")
    }

    tlsCfg := &tls.Config{
        MinVersion: tls.VersionTLS12,
        ServerName: extractServerName(c.cfg.ServerAddr),
    }
    // ...
}
```

Key security features:
- **TLS 1.2 minimum version**: `MinVersion: tls.VersionTLS12`
- **ServerName verification**: Automatically extracts hostname from server address for TLS verification
- **Refuse insecure connections**: Returns error when no certificates are configured, does not allow downgrade to plaintext

### 4.2 mTLS (Optional)

Supports mutual TLS authentication, requiring the following three fields to be configured:

```yaml
grpc:
  mtls:
    cert_file: "/path/to/client.crt"   # Client certificate
    key_file: "/path/to/client.key"    # Client private key
    ca_file: "/path/to/ca.crt"         # CA certificate
```

When only `ca_file` is configured without client certificates, the system CA is used for server verification.

---

## 5. Plugin Security

### 5.1 Binary Path Traversal Protection

The `binary_path` in the plugin manifest undergoes path traversal checking to ensure the resolved path does not escape the plugin directory:

```go
// internal/pluginruntime/manifest.go
func LoadManifest(path string) (*PluginManifest, error) {
    // ...
    cleanBin := filepath.Clean(m.BinaryPath)
    cleanDir := filepath.Clean(m.resolvedDir)
    if !strings.HasPrefix(cleanBin, cleanDir+string(filepath.Separator)) {
        return nil, fmt.Errorf("binary_path %q escapes plugin directory %q", m.BinaryPath, m.resolvedDir)
    }
    // ...
}
```

### 5.2 Plugin Name Sanitization

Plugin names only allow letters, numbers, hyphens, and underscores, and must not start with a hyphen:

```go
// internal/pluginruntime/gateway.go
var validPluginName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func sanitizePluginName(name string) (string, error) {
    if !validPluginName.MatchString(name) {
        return "", fmt.Errorf("invalid plugin name %q: must be alphanumeric with hyphens/underscores", name)
    }
    return name, nil
}
```

### 5.3 Socket Path Security

Plugin sockets use a dedicated directory with 0700 permissions, and defend against symlink attacks:

```go
// internal/pluginruntime/gateway.go
socketDir := filepath.Join(os.TempDir(), "opsagent-plugins")
os.MkdirAll(socketDir, 0o700)

socketPath := filepath.Join(socketDir, safeName+".sock")

// Defend against symlink/TOCTOU attacks
if fi, err := os.Lstat(socketPath); err == nil {
    if fi.Mode()&os.ModeSymlink != 0 {
        return fmt.Errorf("socket path %q is a symlink, refusing to remove", socketPath)
    }
    _ = os.Remove(socketPath)
}
```

### 5.4 FsScan Path Whitelist

The filesystem scan plugin restricts scan paths to safe directories:

```rust
// rust-runtime/src/handlers/fs_scan.rs
const ALLOWED_ROOTS: &[&str] = &["/var/log", "/opt", "/srv", "/tmp"];

// Validate path
let canonical = std::fs::canonicalize(root_path)?;
let is_allowed = ALLOWED_ROOTS.iter().any(|allowed| canonical.starts_with(allowed));
if !is_allowed {
    return Err(PluginError::Config(format!(
        "root_path {} is not under allowed roots: {:?}", root_path, ALLOWED_ROOTS
    )));
}
```

---

## 6. Data Security

### 6.1 Secret Masking in Configuration Diffs

During configuration hot reload, sensitive fields in the diff report are automatically masked:

```go
// internal/config/diff.go
func maskSecret(s string) string {
    if len(s) <= 4 {
        return "***"
    }
    return s[:2] + "***" + s[len(s)-2:]
}

// Usage example: gRPC enrollment token
changes = append(changes, NonReloadableChange{
    Field:  "grpc.enroll_token",
    OldVal: maskSecret(old.GRPC.EnrollToken),
    NewVal: maskSecret(new.GRPC.EnrollToken),
})
```

### 6.2 File Permissions

All sensitive files use strict permissions:

| File Type | Permission | Code Location |
|----------|------|----------|
| Configuration file | 0600 | Packaging script `scripts/ci-package.sh` |
| Audit log | 0600 | `internal/sandbox/audit.go`, `internal/app/audit.go` |
| Temporary script file | 0600 | `internal/sandbox/nsjail.go` |
| Temporary config file | 0600 | `internal/sandbox/nsjail.go` |
| Cache persistence file | 0600 | `internal/grpcclient/client.go` |

### 6.3 Unpredictable Temporary Paths

Script and configuration temporary files use random names to prevent attackers from predicting paths:

```go
// internal/sandbox/nsjail.go
func (c *NsjailConfig) WriteScriptFile(taskID, scriptContent string) (string, error) {
    scriptDir := filepath.Join(os.TempDir(), "nsjail-scripts")
    os.MkdirAll(scriptDir, 0o700)
    f, err := os.CreateTemp(scriptDir, "task-*.sh")  // Random suffix
    // ...
    os.Chmod(f.Name(), 0o600)
    // ...
}
```

Tests verify path unpredictability:

```go
// internal/sandbox/sanitize_test.go
func TestWriteScriptFileUnpredictablePath(t *testing.T) {
    path1, _ := cfg.WriteScriptFile("task-1", "echo hello")
    path2, _ := cfg.WriteScriptFile("task-1", "echo hello")
    if path1 == path2 {
        t.Errorf("expected unpredictable paths, got same path twice")
    }
}
```

### 6.4 dmesg Output Limitation

The local probe plugin limits dmesg output size to 64KB when reading to prevent memory exhaustion:

```rust
// rust-runtime/src/handlers/local_probe.rs
let truncated = if stdout.len() > 65536 {
    &stdout[stdout.len() - 65536..]  // Only check the last 64KB
} else {
    &stdout
};
```

---

## 7. System Security

### 7.1 systemd Service Hardening

The systemd service unit implements the following security restrictions:

```ini
# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/log/opsagent /tmp/opsagent
ProtectHome=true
PrivateTmp=true
```

| Directive | Purpose |
|------|------|
| `NoNewPrivileges=true` | Prohibit process privilege escalation (e.g., setuid, setgid) |
| `ProtectSystem=strict` | Read-only filesystem, only `ReadWritePaths` are writable |
| `ProtectHome=true` | Hide `/home`, `/root`, `/run/user` |
| `PrivateTmp=true` | Use private `/tmp`, isolating temporary files from other processes |

### 7.2 Localhost Binding

HTTP server and Prometheus output bind to `127.0.0.1` by default, only accepting local connections:

```go
// internal/config/config.go
v.SetDefault("server.listen_addr", "127.0.0.1:18080")
```

```go
// internal/collector/outputs/prometheus/prometheus.go
const defaultAddr = "127.0.0.1:9100"
```

### 7.3 HTTP ReadHeaderTimeout

HTTP server sets `ReadHeaderTimeout` to prevent Slowloris attacks:

```go
// internal/server/server.go
s.httpServer = &http.Server{
    Addr:              listenAddr,
    Handler:           s.withMiddleware(mux),
    ReadHeaderTimeout: 5 * time.Second,
}
```

---

## 8. Audit Logging

### 8.1 Application-Level Audit Logging

Application-level audit logging uses JSON-lines format with log rotation support:

```go
// internal/app/audit.go
type AuditEvent struct {
    Timestamp time.Time              `json:"timestamp"`
    EventType string                 `json:"event_type"`
    Component string                 `json:"component"`
    Action    string                 `json:"action"`
    Status    string                 `json:"status"`
    Details   map[string]interface{} `json:"details,omitempty"`
    Error     string                 `json:"error,omitempty"`
}
```

Log rotation uses lumberjack:

```go
// internal/app/audit.go
func NewAuditLogger(path string, maxSizeMB, maxBackups int) (*AuditLogger, error) {
    lj := &lumberjack.Logger{
        Filename:   path,
        MaxSize:    maxSizeMB,
        MaxBackups: maxBackups,
        Compress:   true,
    }
    return &AuditLogger{logger: lj}, nil
}
```

**Audit Event Types**:

| Event Type | Component | Description |
|----------|------|------|
| `config.loaded` | agent | Configuration loaded |
| `config.reloaded` | agent | Configuration hot reload success |
| `config.rejected` | agent | Configuration hot reload failure |
| `agent.started` | agent | Agent started |
| `agent.shutting_down` | agent | Agent shutting down |
| `agent.stopped` | agent | Agent stopped |
| `plugin.started` | pluginruntime | Plugin runtime started |
| `plugin.stopped` | pluginruntime | Plugin runtime stopped |
| `grpc.connected` | grpcclient | gRPC connection established |
| `grpc.disconnected` | grpcclient | gRPC connection disconnected |
| `task.started` | dispatcher | Task started |
| `task.completed` | dispatcher | Task completed |
| `task.failed` | dispatcher | Task failed |
| `task.cancelled` | dispatcher | Task cancelled |
| `sandbox.executed` | sandbox | Sandbox execution completed |

### 8.2 Sandbox Audit Logging

The sandbox subsystem maintains an independent audit log recording detailed information for each execution:

```go
// internal/sandbox/audit.go
type AuditEvent struct {
    TaskID      string        `json:"task_id"`
    Timestamp   time.Time     `json:"timestamp"`
    TriggeredBy string        `json:"triggered_by"`
    Type        string        `json:"type"`
    Command     string        `json:"command"`
    Interpreter string        `json:"interpreter,omitempty"`
    ExitCode    int           `json:"exit_code"`
    Duration    time.Duration `json:"duration"`
    TimedOut    bool          `json:"timed_out"`
    Truncated   bool          `json:"truncated"`
    Killed      bool          `json:"killed"`
    Stats       *Stats        `json:"stats,omitempty"`
    Error       string        `json:"error,omitempty"`
}
```

Sandbox audit log file permissions are 0600:

```go
// internal/sandbox/audit.go
f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
```

---

## 9. Security Configuration Reference

Below are the key security-related fields in `config.yaml`:

```yaml
agent:
  id: "agent-local-001"
  name: "local-dev-agent"
  interval_seconds: 10
  shutdown_timeout_seconds: 30
  audit_log:
    enabled: false                    # Recommended to enable in production
    path: "/var/log/opsagent/audit.jsonl"
    max_size_mb: 100
    max_backups: 5

server:
  listen_addr: "127.0.0.1:18080"     # Default localhost only

auth:
  enabled: true                       # Enabled by default
  bearer_token: ""                    # Must set 32+ character strong token in production

prometheus:
  enabled: true
  path: "/metrics"
  protect_with_auth: false            # Set to true to protect metrics endpoint

executor:
  timeout_seconds: 10
  max_output_bytes: 65536
  allowed_commands:
    - uptime
    - df
    - free
    - whoami
    - hostname
    - ip
    - ss

sandbox:
  enabled: false                      # Disabled by default, must be explicitly enabled
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
    blocked_commands:
      - rm
      - mkfs
      - dd
    blocked_keywords:
      - "rm -rf /"
    allowed_interpreters:
      - bash
      - python3
    script_max_bytes: 65536
    shell_injection_check: true        # Enabled by default

grpc:
  server_addr: "platform.example.com:443"
  enroll_token: ""
  mtls:
    cert_file: ""                     # Client certificate path
    key_file: ""                      # Client private key path
    ca_file: ""                       # CA certificate path
  heartbeat_interval_seconds: 15
  reconnect_initial_backoff_ms: 1000
  reconnect_max_backoff_ms: 30000

plugin:
  enabled: false
  runtime_path: "./rust-runtime/target/release/opsagent-rust-runtime"
  socket_path: "/tmp/opsagent/plugin.sock"
  auto_start: true
  startup_timeout_seconds: 5
  request_timeout_seconds: 30
  max_concurrent_tasks: 4
  max_result_bytes: 8388608           # 8MB
  chunk_size_bytes: 262144            # 256KB
  sandbox_profile: "strict"

plugin_gateway:
  enabled: false
  plugins_dir: "/etc/opsagent/plugins"
  startup_timeout_seconds: 10
  health_check_interval_seconds: 30
  max_restarts: 3
  restart_backoff_seconds: 5
  file_watch_debounce_seconds: 2
```

---

## 10. Security Checklist

Please confirm the following security configurations item by item before deployment:

- [ ] `auth.enabled` set to `true`
- [ ] `bearer_token` set to a strong random value of 32+ characters
- [ ] `server.listen_addr` bound to `127.0.0.1` (or specific network interface address)
- [ ] Configure gRPC mTLS in production (`grpc.mtls.cert_file`, `key_file`, `ca_file`)
- [ ] `sandbox.policy.allowed_commands` follows principle of least privilege
- [ ] `sandbox.policy.blocked_commands` includes dangerous commands like `rm`, `mkfs`, `dd`
- [ ] `agent.audit_log.enabled` set to `true`
- [ ] Configuration file permissions are `0600`
- [ ] systemd service uses `NoNewPrivileges=true`
- [ ] systemd service uses `ProtectSystem=strict` and `ProtectHome=true`
- [ ] Plugin binaries come from trusted sources
- [ ] Regularly execute `make security` (gosec static security scan)
- [ ] Regularly execute `make test-race` (race condition detection)
- [ ] `prometheus.protect_with_auth` set to `true` when needed
- [ ] Audit log path is writable with correct permissions
- [ ] Sandbox cgroup base path exists and is writable
