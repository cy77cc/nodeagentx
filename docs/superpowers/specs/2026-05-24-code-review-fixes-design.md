# Code Review Fixes Design

**Date:** 2026-05-24
**Scope:** 35 issues (4 CRITICAL, 8 HIGH, 14 MEDIUM, 9 LOW)
**Approach:** Targeted per-issue fixes, priority order
**Organization:** Split by module

## CRITICAL Fixes (4)

### C1: Replace gateway stub types
**File:** `internal/gateway/gateway.go:299-329`
**Problem:** Stub `TunnelPool` and `Tunnel` types shadow real implementations in `internal/gateway/tunnel/`.

**Fix:**
- Delete stub types (`TunnelPool`, `NewTunnelPool`, `Tunnel`, `NewTunnel`, all stub methods)
- Change `Gateway.pool` field type from `*TunnelPool` to `*tunnel.Pool`
- Update `New()` to use `tunnel.NewPool(cfg.MaxTunnels)`
- Update `handleIncoming()` to use `tunnel.NewTunnel(tunnelID, conn, g.tunnelSender, ...)`
- Import `github.com/cy77cc/opsagent/internal/gateway/tunnel`

### C2: SSH host key verification
**File:** `internal/gateway/proxy/ssh.go:44`
**Problem:** `ssh.InsecureIgnoreHostKey()` accepts any key, enabling MITM.

**Fix:**
- Add `KnownHostsFile string` to `SSHConfig`
- Add `InsecureSkipVerify bool` to `SSHConfig`
- If `InsecureSkipVerify` is true, use `ssh.InsecureIgnoreHostKey()` with a warning log
- Otherwise, use `knownhosts.New(knownHostsFile)` as `HostKeyCallback`
- Default `knownHostsFile` to `~/.ssh/known_hosts`
- Add `golang.org/x/crypto/ssh/knownhosts` import

### C3: Fix metric persistence
**File:** `internal/grpcclient/client.go:519,621`
**Problem:** `json.Marshal` on `[]*collector.Metric` produces `[{},{}]` due to unexported fields.

**Fix:**
- In `FlushAndStop()`: replace `json.Marshal(metrics)` with `persistMetrics(metrics, persistPath)`
- In `loadPersistedCache()`: replace `json.Unmarshal` with `loadMetrics(path)`
- Add file size check (10MB default) before loading (addresses H7)

### C4: Environment variable whitelist
**File:** `internal/sandbox/executor.go:341-354`
**Problem:** Blacklist only blocks 3 variables; missing LD_AUDIT, GLIBC_TUNABLES, BASH_ENV, etc.

**Fix:**
- Switch to explicit allowlist mode
- Safe vars: `PATH`, `HOME`, `LANG`, `TERM`, `USER`, `SHELL`, `HOSTNAME`, `TMPDIR`
- Only pass request env vars that are in the allowlist
- Log blocked variables at debug level

## HIGH Fixes (8)

### H1: Complete shell metacharacter list
**File:** `internal/sandbox/policy.go:108-126`
**Problem:** Missing `!`, `(`, `)`, `[`, `]`, `{`, `}`.

**Fix:** Add these to `shellMetachars` slice.

### H2: SSH command argument escaping
**File:** `internal/gateway/proxy/ssh.go:75-78`
**Problem:** `fullCmd += " " + arg` allows shell metacharacter injection.

**Fix:**
- Implement shell-safe quoting: wrap each arg in single quotes, escape embedded single quotes as `'\''`
- Create helper function `shellQuote(s string) string`
- Apply in `Execute()` method

### H3: TCP listener authentication
**File:** `internal/gateway/gateway.go:198-214`
**Problem:** Any client that can reach the port can establish a tunnel.

**Fix:**
- Add `AuthPSK string` to gateway `Config`
- In `handleIncoming()`, if PSK is set:
  - Read first 32 bytes from connection
  - Compare against PSK using `subtle.ConstantTimeCompare`
  - Close connection on mismatch
- If PSK is empty, log warning (backward compatible)

### H4: Gateway constructor panic
**File:** `internal/gateway/gateway.go:49-55`
**Problem:** `New()` panics on nil args.

**Fix:**
- Change signature to `func New(...) (*Gateway, error)`
- Return `nil, fmt.Errorf(...)` instead of `panic()`
- Update caller in `internal/app/agent.go:265`

### H5: Plugin runtime env leak
**File:** `internal/pluginruntime/runtime.go:102`
**Problem:** `cmd.Env = os.Environ()` leaks all host secrets to plugin process.

**Fix:**
- Create `buildPluginRuntimeEnv()` similar to sandbox's `buildSandboxEnv()`
- Allowlist: PATH, HOME, LANG, TERM, OPSAGENT_SOCKET_PATH
- Block: LD_PRELOAD, LD_LIBRARY_PATH, DYLD_INSERT_LIBRARIES

### H6: SendProxyMetrics dead code
**File:** `internal/grpcclient/client.go:603-613`
**Problem:** Returns nil without sending anything.

**Fix:**
- Build a `ProxyMetricBatch` message (proto type exists)
- Send via `stream.Send(msg)` with proper error handling

### H7: loadPersistedCache size limit
**File:** `internal/grpcclient/client.go:617`
**Problem:** No file size check, potential OOM.

**Fix:** Addressed alongside C3 - add `os.Stat()` check before reading, reject files > 10MB.

### H8: Timeout upper bound
**File:** `internal/sandbox/executor.go:308-326`
**Problem:** No max timeout allows 24-hour executions.

**Fix:**
- Add `MaxTimeoutSec int` to `Config` (default: `TimeoutSec * 10`)
- Cap `req.Timeout` in `buildNsjailConfig()`
- Log warning when timeout is capped

## MEDIUM Fixes (14)

### M1: Flush output ordering
**File:** `internal/sandbox/output_streamer.go:54-63`
**Fix:** Hold lock during `sender()` call. Sender is fast (buffer write), acceptable trade-off.

### M2: Empty allowlist silent drop
**File:** `internal/sandbox/executor.go:308-326`
**Fix:** If `NetworkMode == "allowlist"` and `len(AllowedIPs) == 0`, log warning and switch to "disabled".

### M3: Heartbeat in messageLoop
**File:** `internal/grpcclient/client.go:313-321`
**Fix:** Remove `default` case; run heartbeat in a separate goroutine.

### M4: Timer leak in connectLoop
**File:** `internal/grpcclient/client.go:229-234`
**Fix:** Use `time.NewTimer()` with explicit `timer.Stop()`.

### M5: replayCache gRPC size limit
**File:** `internal/grpcclient/client.go:372-394`
**Fix:** Split into batches of 100, send each separately.

### M6: FlushAndStop race
**File:** `internal/grpcclient/client.go:486-538`
**Fix:** Drain cache before canceling context.

### M7: Shutdown TOCTOU
**File:** `internal/app/agent.go:613-632`
**Fix:** Use mutex to atomically check shutdown state and store task.

### M8: Silent fallback to unsandboxed
**File:** `internal/app/agent.go:949-972`
**Fix:** Add `AllowUnsandboxedFallback bool` config (default false). Log warning on fallback. Return error if not allowed.

### M9: Extract task handler middleware
**File:** `internal/app/agent.go:607-916`
**Fix:** Extract `withTaskMiddleware(taskType, handler)` that wraps audit, metrics, error handling.

### M10: Gateway interface compile check
**File:** `internal/app/interfaces.go:87-94`
**Fix:** Add `var _ Gateway = (*gateway.Gateway)(nil)`.

### M11: Hot-reload PluginGateway/Checker
**File:** `internal/config/diff.go:25-78`
**Fix:** Add `PluginGatewayChanged` and `CheckerChanged` to `ChangeSet`. Add diff functions. Wire reloaders.

### M12: Rate limiter eviction
**File:** `internal/server/middleware.go:13-37`
**Fix:** Track last-seen time per visitor. Background goroutine evicts stale entries every 5 minutes.

### M13: DropOldest memory leak
**File:** `internal/collector/buffer.go:42`
**Fix:** Set `b.metrics[0] = nil` before slicing to allow GC.

### M14: healthCheckAll stale pointers
**File:** `internal/pluginruntime/gateway.go:640-704`
**Fix:** After restart attempt, re-read plugin from `g.plugins[name]` map instead of using stale `p`.

## LOW Fixes (9)

### L1: hostID/hostname confusion
**File:** `internal/gateway/gateway.go:87`
**Fix:** Add `Hostname string` to `HostConfig`. Use `h.Hostname` for register, fall back to `h.ID`.

### L2: Predictable tunnel ID
**File:** `internal/gateway/gateway.go:225`
**Fix:** Use `crypto/rand` for random suffix. Format: `tunnel-<hex>` (8 random bytes).

### L3: Multiple net.Conn.Close()
**File:** `internal/gateway/tunnel/tunnel.go:70-89`
**Fix:** Remove `defer t.conn.Close()` from `Relay()`. Let `Close()` be sole owner.

### L4: ExecuteMetricsCollect dead code
**File:** `internal/gateway/proxy/proxy.go:143-144`
**Fix:** Parse collected metrics into structured format and call `m.sender.SendProxyMetrics()`.

### L5: FlushAndStop ignores ctx
**File:** `internal/grpcclient/client.go:486`
**Fix:** Use `ctx` for timeout on flush send operations.

### L6: metricTypeString missing Histogram
**File:** `internal/grpcclient/persist.go:69-88`
**Fix:** Add `case collector.Histogram: return "histogram"` and corresponding `metricTypeFromString` case. Histogram type exists (`MetricType = 2`).

### L7: python → python3 mapping
**File:** `internal/sandbox/nsjail.go:274-275`
**Fix:** Log deprecation warning when "python" is used.

### L8: ToArgs nil check
**File:** `internal/sandbox/nsjail.go:74-78`
**Fix:** `CommandArgs()` should check if `args` is nil after `ToArgs()` call.

### L9: Hardcoded 30s timeout
**File:** `internal/app/agent.go:1155`
**Fix:** Use `a.cfg.Plugin.RequestTimeoutSeconds` instead of hardcoded 30s.

## Module Mapping

| Module | Issues | Files |
|--------|--------|-------|
| gateway | C1, H3, H4, L1, L2, L3 | gateway.go, tunnel/tunnel.go, config.go |
| gateway/proxy | C2, H2, L4 | ssh.go, proxy.go |
| grpcclient | C3, H6, H7, M3, M4, M5, M6, L5, L6 | client.go, persist.go |
| sandbox | C4, H1, H8, M1, M2, L7, L8 | executor.go, policy.go, nsjail.go, output_streamer.go |
| app | M7, M8, M9, M10, L9 | agent.go, interfaces.go |
| config | M11 | diff.go |
| server | M12 | middleware.go |
| collector | M13 | buffer.go |
| pluginruntime | H5, M14 | runtime.go, gateway.go |

## Implementation Order

1. **Phase 1 - CRITICAL:** C1, C2, C3, C4
2. **Phase 2 - HIGH:** H1, H2, H3, H4, H5, H6, H7, H8
3. **Phase 3 - MEDIUM:** M1-M14
4. **Phase 4 - LOW:** L1-L9

Each phase produces a separate commit (or PR if desired).
