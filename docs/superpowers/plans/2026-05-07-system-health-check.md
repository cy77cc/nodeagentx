# System Health Check Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement platform-triggered host health checks with typed checkers, streaming results via gRPC.

**Architecture:** Checker interface + registry pattern (mirrors collector plugins). Platform sends `HealthCheckRequest` via gRPC, agent routes each `CheckItem` to a typed `Checker`, streams `HealthCheckResult` back. 20 built-in checkers across 5 categories.

**Tech Stack:** Go 1.26.1, protobuf, gRPC, zerolog, gopsutil/v4

**Spec:** `docs/superpowers/specs/2026-05-07-system-health-check-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `proto/agent.proto` | gRPC message definitions for health check |
| `internal/checker/types.go` | CheckStatus, CheckSeverity, CheckResult, Checker interface |
| `internal/checker/registry.go` | Registry, DefaultRegistry, Register, Get, Types |
| `internal/checker/executor.go` | Executor: routes check items to checkers, streams results |
| `internal/checker/types_test.go` | Tests for types and protobuf conversion |
| `internal/checker/registry_test.go` | Tests for registry CRUD |
| `internal/checker/executor_test.go` | Tests for executor with mock checkers |
| `internal/checker/kernel/sysctl.go` | Sysctl checker |
| `internal/checker/kernel/kernel_version.go` | Kernel version checker |
| `internal/checker/kernel/kernel_module.go` | Kernel module checker |
| `internal/checker/kernel/boot_param.go` | Boot parameter checker |
| `internal/checker/kernel/kernel_test.go` | Tests for all kernel checkers |
| `internal/checker/filesystem/file_perm.go` | File permission checker |
| `internal/checker/filesystem/file_exist.go` | File existence checker |
| `internal/checker/filesystem/dir_perm.go` | Directory permission checker |
| `internal/checker/filesystem/mount_option.go` | Mount option checker |
| `internal/checker/filesystem/filesystem_test.go` | Tests for all filesystem checkers |
| `internal/checker/network/port.go` | Port listening checker |
| `internal/checker/network/ssh_config.go` | SSH config checker |
| `internal/checker/network/iptables.go` | Iptables rule checker |
| `internal/checker/network/network_param.go` | Network sysctl checker |
| `internal/checker/network/network_test.go` | Tests for all network checkers |
| `internal/checker/service/service.go` | Systemd service checker |
| `internal/checker/service/user.go` | User account checker |
| `internal/checker/service/cron.go` | Cron job checker |
| `internal/checker/service/pam.go` | PAM config checker |
| `internal/checker/service/service_test.go` | Tests for all service checkers |
| `internal/checker/container/docker.go` | Docker config checker |
| `internal/checker/container/containerd.go` | containerd config checker |
| `internal/checker/container/cgroup.go` | Cgroup checker |
| `internal/checker/container/runtime.go` | Container runtime checker |
| `internal/checker/container/container_test.go` | Tests for all container checkers |
| `internal/config/config.go` | Add CheckerConfig (modify) |
| `internal/grpcclient/receiver.go` | Add HealthCheckHandler (modify) |
| `internal/grpcclient/sender.go` | Add NewHealthCheckResultMessage (modify) |
| `internal/grpcclient/client.go` | Add SendHealthCheckResult (modify) |
| `internal/app/interfaces.go` | Add SendHealthCheckResult to GRPCClient interface (modify) |
| `internal/app/agent.go` | Wire checker executor + gRPC handler + blank imports (modify) |

---

## Task 1: Proto Definition

**Files:**
- Modify: `proto/agent.proto`
- Regenerate: `internal/grpcclient/proto/agent.pb.go`, `internal/grpcclient/proto/agent_grpc.pb.go`

- [ ] **Step 1: Add health check messages to agent.proto**

Add the following after the existing `Ack` message (line 151) in `proto/agent.proto`:

```protobuf
message HealthCheckRequest {
  string request_id = 1;
  repeated CheckItem items = 2;
  int32 timeout_seconds = 3;
}

message CheckItem {
  string id = 1;
  string type = 2;
  string category = 3;
  string name = 4;
  string description = 5;
  bytes params = 6;
  CheckSeverity severity = 7;
}

enum CheckSeverity {
  SEVERITY_INFO = 0;
  SEVERITY_LOW = 1;
  SEVERITY_MEDIUM = 2;
  SEVERITY_HIGH = 3;
  SEVERITY_CRITICAL = 4;
}

message HealthCheckResult {
  string request_id = 1;
  repeated CheckResult results = 2;
  CheckSummary summary = 3;
  bool completed = 4;
}

message CheckResult {
  string item_id = 1;
  string type = 2;
  string name = 3;
  CheckStatus status = 4;
  string actual_value = 5;
  string expected_value = 6;
  string message = 7;
  string remediation = 8;
  CheckSeverity severity = 9;
  int64 duration_ms = 10;
}

enum CheckStatus {
  STATUS_PASS = 0;
  STATUS_FAIL = 1;
  STATUS_WARN = 2;
  STATUS_ERROR = 3;
  STATUS_SKIP = 4;
}

message CheckSummary {
  int32 total = 1;
  int32 pass = 2;
  int32 fail = 3;
  int32 warn = 4;
  int32 error = 5;
  int32 skip = 6;
  int64 total_duration_ms = 7;
}
```

Add `HealthCheckRequest health_check = 6;` to the `PlatformMessage.oneof` block, and `HealthCheckResult health_check_result = 7;` to the `AgentMessage.oneof` block.

- [ ] **Step 2: Regenerate protobuf code**

Run:
```bash
cd /root/project/opsagent
protoc --go_out=. --go-grpc_out=. proto/agent.proto
```

- [ ] **Step 3: Verify build compiles**

Run:
```bash
go build ./...
```
Expected: clean build

- [ ] **Step 4: Commit**

```bash
git add proto/agent.proto internal/grpcclient/proto/
git commit -m "feat(checker): add health check proto messages"
```

---

## Task 2: Checker Types and Interface

**Files:**
- Create: `internal/checker/types.go`
- Create: `internal/checker/types_test.go`

- [ ] **Step 1: Write types_test.go**

```go
package checker

import (
	"encoding/json"
	"testing"
	"time"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckStatusToProto(t *testing.T) {
	tests := []struct {
		input    CheckStatus
		expected pb.CheckStatus
	}{
		{StatusPass, pb.CheckStatus_STATUS_PASS},
		{StatusFail, pb.CheckStatus_STATUS_FAIL},
		{StatusWarn, pb.CheckStatus_STATUS_WARN},
		{StatusError, pb.CheckStatus_STATUS_ERROR},
		{StatusSkip, pb.CheckStatus_STATUS_SKIP},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.input.ToProto())
	}
}

func TestCheckSeverityToProto(t *testing.T) {
	tests := []struct {
		input    CheckSeverity
		expected pb.CheckSeverity
	}{
		{SeverityInfo, pb.CheckSeverity_SEVERITY_INFO},
		{SeverityLow, pb.CheckSeverity_SEVERITY_LOW},
		{SeverityMedium, pb.CheckSeverity_SEVERITY_MEDIUM},
		{SeverityHigh, pb.CheckSeverity_SEVERITY_HIGH},
		{SeverityCritical, pb.CheckSeverity_SEVERITY_CRITICAL},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.input.ToProto())
	}
}

func TestCheckResultToProto(t *testing.T) {
	r := &CheckResult{
		ItemID:        "test-1",
		Type:          "sysctl_check",
		Name:          "IP Forward",
		Status:        StatusFail,
		ActualValue:   "1",
		ExpectedValue: "0",
		Message:       "ip_forward is enabled",
		Remediation:   "Set net.ipv4.ip_forward=0",
		Severity:      SeverityHigh,
		Duration:      50 * time.Millisecond,
	}

	proto := r.ToProto()
	assert.Equal(t, "test-1", proto.ItemId)
	assert.Equal(t, "sysctl_check", proto.Type)
	assert.Equal(t, pb.CheckStatus_STATUS_FAIL, proto.Status)
	assert.Equal(t, int64(50), proto.DurationMs)
}

func TestBuildSummary(t *testing.T) {
	results := []*CheckResult{
		{Status: StatusPass},
		{Status: StatusPass},
		{Status: StatusFail},
		{Status: StatusWarn},
		{Status: StatusError},
		{Status: StatusSkip},
	}
	summary := BuildSummary(results, 500*time.Millisecond)
	assert.Equal(t, int32(6), summary.Total)
	assert.Equal(t, int32(2), summary.Pass)
	assert.Equal(t, int32(1), summary.Fail)
	assert.Equal(t, int32(1), summary.Warn)
	assert.Equal(t, int32(1), summary.Error)
	assert.Equal(t, int32(1), summary.Skip)
	assert.Equal(t, int64(500), summary.TotalDurationMs)
}

func TestCheckerInterface(t *testing.T) {
	// Verify a mock checker satisfies the interface.
	var _ Checker = &mockChecker{}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/checker/ -v -run TestCheckStatusToProto
```
Expected: FAIL — package does not exist

- [ ] **Step 3: Write types.go**

```go
package checker

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
)

// CheckStatus represents the outcome of a single check.
type CheckStatus int

const (
	StatusPass  CheckStatus = iota // Check passed
	StatusFail                      // Check failed
	StatusWarn                      // Warning — may need attention
	StatusError                     // Error executing check
	StatusSkip                      // Check skipped (e.g., not applicable)
)

// ToProto converts CheckStatus to its protobuf enum.
func (s CheckStatus) ToProto() pb.CheckStatus {
	switch s {
	case StatusPass:
		return pb.CheckStatus_STATUS_PASS
	case StatusFail:
		return pb.CheckStatus_STATUS_FAIL
	case StatusWarn:
		return pb.CheckStatus_STATUS_WARN
	case StatusError:
		return pb.CheckStatus_STATUS_ERROR
	case StatusSkip:
		return pb.CheckStatus_STATUS_SKIP
	default:
		return pb.CheckStatus_STATUS_ERROR
	}
}

// CheckSeverity indicates the risk level of a check finding.
type CheckSeverity int

const (
	SeverityInfo     CheckSeverity = iota // Informational
	SeverityLow                           // Low risk
	SeverityMedium                        // Medium risk
	SeverityHigh                          // High risk
	SeverityCritical                      // Critical risk
)

// ToProto converts CheckSeverity to its protobuf enum.
func (s CheckSeverity) ToProto() pb.CheckSeverity {
	switch s {
	case SeverityInfo:
		return pb.CheckSeverity_SEVERITY_INFO
	case SeverityLow:
		return pb.CheckSeverity_SEVERITY_LOW
	case SeverityMedium:
		return pb.CheckSeverity_SEVERITY_MEDIUM
	case SeverityHigh:
		return pb.CheckSeverity_SEVERITY_HIGH
	case SeverityCritical:
		return pb.CheckSeverity_SEVERITY_CRITICAL
	default:
		return pb.CheckSeverity_SEVERITY_INFO
	}
}

// CheckResult holds the outcome of a single check item.
type CheckResult struct {
	ItemID        string
	Type          string
	Name          string
	Status        CheckStatus
	ActualValue   string
	ExpectedValue string
	Message       string
	Remediation   string
	Severity      CheckSeverity
	Duration      time.Duration
}

// ToProto converts CheckResult to its protobuf representation.
func (r *CheckResult) ToProto() *pb.CheckResult {
	return &pb.CheckResult{
		ItemId:        r.ItemID,
		Type:          r.Type,
		Name:          r.Name,
		Status:        r.Status.ToProto(),
		ActualValue:   r.ActualValue,
		ExpectedValue: r.ExpectedValue,
		Message:       r.Message,
		Remediation:   r.Remediation,
		Severity:      r.Severity.ToProto(),
		DurationMs:    r.Duration.Milliseconds(),
	}
}

// Checker is the interface for a single health check type.
type Checker interface {
	// Type returns the checker type identifier (e.g., "sysctl_check").
	Type() string

	// Category returns the check category: kernel, filesystem, network, service, container.
	Category() string

	// Check executes the check and returns the result.
	Check(ctx context.Context, params json.RawMessage) (*CheckResult, error)
}

// BuildSummary computes a CheckSummary from a slice of CheckResults.
func BuildSummary(results []*CheckResult, totalDuration time.Duration) *pb.CheckSummary {
	s := &pb.CheckSummary{
		Total:           int32(len(results)),
		TotalDurationMs: totalDuration.Milliseconds(),
	}
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			s.Pass++
		case StatusFail:
			s.Fail++
		case StatusWarn:
			s.Warn++
		case StatusError:
			s.Error++
		case StatusSkip:
			s.Skip++
		}
	}
	return s
}
```

- [ ] **Step 4: Add mock checker for tests**

Add to `internal/checker/types_test.go`:

```go
type mockChecker struct{}

func (m *mockChecker) Type() string     { return "mock" }
func (m *mockChecker) Category() string { return "test" }
func (m *mockChecker) Check(_ context.Context, _ json.RawMessage) (*CheckResult, error) {
	return &CheckResult{Status: StatusPass}, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
go test ./internal/checker/ -v -count=1
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/checker/types.go internal/checker/types_test.go
git commit -m "feat(checker): add types, interfaces, and proto conversions"
```

---

## Task 3: Checker Registry

**Files:**
- Create: `internal/checker/registry.go`
- Create: `internal/checker/registry_test.go`

- [ ] **Step 1: Write registry_test.go**

```go
package checker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mc := &mockChecker{}
	r.Register(mc)

	got, ok := r.Get("mock")
	require.True(t, ok)
	assert.Equal(t, mc, got)
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	assert.False(t, ok)
}

func TestRegistryTypes(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockChecker{})
	r.Register(&mockChecker2{})

	types := r.Types()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "mock")
	assert.Contains(t, types, "mock2")
}

func TestDefaultRegistry(t *testing.T) {
	// Save and restore
	orig := DefaultRegistry
	defer func() { DefaultRegistry = orig }()

	DefaultRegistry = NewRegistry()
	Register(&mockChecker{})
	got, ok := DefaultRegistry.Get("mock")
	require.True(t, ok)
	assert.Equal(t, "mock", got.Type())
}

type mockChecker2 struct{}

func (m *mockChecker2) Type() string     { return "mock2" }
func (m *mockChecker2) Category() string { return "test" }
func (m *mockChecker2) Check(_ context.Context, _ json.RawMessage) (*CheckResult, error) {
	return &CheckResult{Status: StatusPass}, nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/checker/ -v -run TestRegistryRegisterAndGet
```
Expected: FAIL — NewRegistry undefined

- [ ] **Step 3: Write registry.go**

```go
package checker

import (
	"slices"
	"sync"
)

// Registry holds registered Checker instances keyed by type.
type Registry struct {
	mu       sync.RWMutex
	checkers map[string]Checker
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{checkers: make(map[string]Checker)}
}

// Register adds a Checker to the registry.
func (r *Registry) Register(c Checker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[c.Type()] = c
}

// Get returns the Checker for the given type.
func (r *Registry) Get(typ string) (Checker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.checkers[typ]
	return c, ok
}

// Types returns sorted registered checker type names.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.checkers))
	for k := range r.checkers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// DefaultRegistry is the global checker registry.
var DefaultRegistry = NewRegistry()

// Register adds a Checker to the default registry.
func Register(c Checker) {
	DefaultRegistry.Register(c)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/checker/ -v -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/checker/registry.go internal/checker/registry_test.go
git commit -m "feat(checker): add registry with init() registration pattern"
```

---

## Task 4: Checker Executor

**Files:**
- Create: `internal/checker/executor.go`
- Create: `internal/checker/executor_test.go`

- [ ] **Step 1: Write executor_test.go**

```go
package checker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorExecuteAllPass(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&alwaysPassChecker{})

	exec := NewExecutor(reg, zerolog.Nop())
	req := &pb.HealthCheckRequest{
		RequestId:     "req-1",
		TimeoutSeconds: 10,
		Items: []*pb.CheckItem{
			{Id: "item-1", Type: "always_pass", Category: "test", Name: "Always Pass"},
		},
	}

	var results []*pb.HealthCheckResult
	err := exec.Execute(context.Background(), req, func(r *pb.HealthCheckResult) {
		results = append(results, r)
	})
	require.NoError(t, err)

	// Should have 1 intermediate + 1 final
	require.Len(t, results, 2)
	assert.False(t, results[0].Completed)
	assert.True(t, results[1].Completed)
	assert.Equal(t, int32(1), results[1].Summary.Total)
	assert.Equal(t, int32(1), results[1].Summary.Pass)
}

func TestExecutorUnknownCheckerType(t *testing.T) {
	reg := NewRegistry()
	exec := NewExecutor(reg, zerolog.Nop())
	req := &pb.HealthCheckRequest{
		RequestId: "req-2",
		Items: []*pb.CheckItem{
			{Id: "item-1", Type: "nonexistent", Category: "test", Name: "Missing"},
		},
	}

	var results []*pb.HealthCheckResult
	err := exec.Execute(context.Background(), req, func(r *pb.HealthCheckResult) {
		results = append(results, r)
	})
	require.NoError(t, err)
	require.Len(t, results, 2) // 1 intermediate + 1 final
	assert.Equal(t, pb.CheckStatus_STATUS_ERROR, results[0].Results[0].Status)
}

type alwaysPassChecker struct{}

func (c *alwaysPassChecker) Type() string     { return "always_pass" }
func (c *alwaysPassChecker) Category() string { return "test" }
func (c *alwaysPassChecker) Check(_ context.Context, _ json.RawMessage) (*CheckResult, error) {
	return &CheckResult{
		Status:   StatusPass,
		Message:  "all good",
		Duration: time.Millisecond,
	}, nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/checker/ -v -run TestExecutorExecuteAllPass
```
Expected: FAIL — NewExecutor undefined

- [ ] **Step 3: Write executor.go**

```go
package checker

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
)

// Executor runs health check requests by routing items to registered checkers.
type Executor struct {
	registry *Registry
	logger   zerolog.Logger
}

// NewExecutor creates an Executor.
func NewExecutor(registry *Registry, logger zerolog.Logger) *Executor {
	return &Executor{registry: registry, logger: logger}
}

// Execute runs all check items in the request, calling callback for each
// intermediate result and once more for the final summary.
func (e *Executor) Execute(ctx context.Context, req *pb.HealthCheckRequest,
	callback func(*pb.HealthCheckResult)) error {

	results := make([]*CheckResult, 0, len(req.Items))
	startAll := time.Now()

	for _, item := range req.Items {
		result := e.executeOne(ctx, item)
		results = append(results, result)

		// Stream intermediate result
		callback(&pb.HealthCheckResult{
			RequestId: req.RequestId,
			Results:   []*pb.CheckResult{result.ToProto()},
			Completed: false,
		})
	}

	// Build and send final result
	protoResults := make([]*pb.CheckResult, len(results))
	for i, r := range results {
		protoResults[i] = r.ToProto()
	}

	callback(&pb.HealthCheckResult{
		RequestId: req.RequestId,
		Results:   protoResults,
		Summary:   BuildSummary(results, time.Since(startAll)),
		Completed: true,
	})

	return nil
}

func (e *Executor) executeOne(ctx context.Context, item *pb.CheckItem) *CheckResult {
	start := time.Now()

	checker, ok := e.registry.Get(item.Type)
	if !ok {
		return &CheckResult{
			ItemID:   item.Id,
			Type:     item.Type,
			Name:     item.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("unknown checker type: %s", item.Type),
			Severity: CheckSeverity(item.Severity),
			Duration: time.Since(start),
		}
	}

	result, err := checker.Check(ctx, item.Params)
	if err != nil {
		e.logger.Error().Err(err).Str("type", item.Type).Str("item_id", item.Id).Msg("checker failed")
		return &CheckResult{
			ItemID:   item.Id,
			Type:     item.Type,
			Name:     item.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("check execution error: %v", err),
			Severity: CheckSeverity(item.Severity),
			Duration: time.Since(start),
		}
	}

	// Fill in metadata from the request item
	result.ItemID = item.Id
	result.Type = item.Type
	result.Name = item.Name
	if result.Duration == 0 {
		result.Duration = time.Since(start)
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/checker/ -v -count=1
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/checker/executor.go internal/checker/executor_test.go
git commit -m "feat(checker): add executor with streaming result delivery"
```

---

## Task 5: Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `configs/config.yaml`

- [ ] **Step 1: Add CheckerConfig struct**

Add after `PluginGatewayConfig` (line 154) in `internal/config/config.go`:

```go
// CheckerConfig controls the system health checker subsystem.
type CheckerConfig struct {
	Enabled              bool     `mapstructure:"enabled"`
	MaxConcurrent        int      `mapstructure:"max_concurrent"`
	DefaultTimeoutSeconds int     `mapstructure:"default_timeout_seconds"`
	DisabledCheckers     []string `mapstructure:"disabled_checkers"`
}
```

- [ ] **Step 2: Add Checker field to Config struct**

Add `Checker CheckerConfig \`mapstructure:"checker"\`` after the `PluginGateway` field (line 22) in the `Config` struct.

- [ ] **Step 3: Add defaults in Load()**

Add after the `plugin_gateway` defaults (line 204):

```go
v.SetDefault("checker.enabled", true)
v.SetDefault("checker.max_concurrent", 5)
v.SetDefault("checker.default_timeout_seconds", 30)
```

- [ ] **Step 4: Add validation in Validate()**

Add before the final `return nil` in `Validate()`:

```go
if c.Checker.Enabled {
    if c.Checker.MaxConcurrent <= 0 {
        return fmt.Errorf("checker.max_concurrent must be > 0 when checker.enabled=true")
    }
    if c.Checker.DefaultTimeoutSeconds <= 0 {
        return fmt.Errorf("checker.default_timeout_seconds must be > 0 when checker.enabled=true")
    }
}
```

- [ ] **Step 5: Add checker config to default config.yaml**

Add at the end of `configs/config.yaml`:

```yaml
checker:
  enabled: true
  max_concurrent: 5
  default_timeout_seconds: 30
  disabled_checkers: []
```

- [ ] **Step 6: Verify build**

Run:
```bash
go build ./...
```
Expected: clean build

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go configs/config.yaml
git commit -m "feat(checker): add checker configuration with validation"
```

---

## Task 6: gRPC Receiver Integration

**Files:**
- Modify: `internal/grpcclient/receiver.go`

- [ ] **Step 1: Add HealthCheckHandler type and field**

Add after `ConfigUpdateHandler` (line 22) in `receiver.go`:

```go
// HealthCheckHandler handles HealthCheckRequest platform messages.
type HealthCheckHandler func(ctx context.Context, req *pb.HealthCheckRequest) error
```

Add field to `Receiver` struct (after `onConfig`):

```go
onHealthCheck HealthCheckHandler
```

- [ ] **Step 2: Add setter method**

Add after `SetConfigUpdateHandler` (line 48):

```go
// SetHealthCheckHandler registers the handler for HealthCheckRequest messages.
func (r *Receiver) SetHealthCheckHandler(h HealthCheckHandler) { r.onHealthCheck = h }
```

- [ ] **Step 3: Add case in Handle() switch**

Add before the `case *pb.PlatformMessage_Ack:` block (line 85):

```go
case *pb.PlatformMessage_HealthCheck:
    r.logger.Info().Str("request_id", p.HealthCheck.GetRequestId()).Msg("received HealthCheckRequest")
    if r.onHealthCheck != nil {
        return r.onHealthCheck(ctx, p.HealthCheck)
    }
    r.logger.Warn().Str("request_id", p.HealthCheck.GetRequestId()).Msg("no health check handler registered")
```

- [ ] **Step 4: Verify build**

Run:
```bash
go build ./...
```
Expected: clean build

- [ ] **Step 5: Commit**

```bash
git add internal/grpcclient/receiver.go
git commit -m "feat(checker): add HealthCheckHandler to gRPC receiver"
```

---

## Task 7: gRPC Sender Integration

**Files:**
- Modify: `internal/grpcclient/sender.go`
- Modify: `internal/grpcclient/client.go`
- Modify: `internal/app/interfaces.go`

- [ ] **Step 1: Add NewHealthCheckResultMessage to sender.go**

Add after `NewConfigUpdateAck` (line 139) in `sender.go`:

```go
// NewHealthCheckResultMessage wraps a HealthCheckResult into an AgentMessage.
func NewHealthCheckResultMessage(result *pb.HealthCheckResult) *pb.AgentMessage {
	return &pb.AgentMessage{
		Payload: &pb.AgentMessage_HealthCheckResult{
			HealthCheckResult: result,
		},
	}
}
```

- [ ] **Step 2: Add SendHealthCheckResult to client.go**

Add after `SendExecResult` (line 172) in `client.go`:

```go
// SendHealthCheckResult sends a health check result to the platform.
func (c *Client) SendHealthCheckResult(result *pb.HealthCheckResult) {
	c.mu.Lock()
	stream := c.stream
	connected := c.connected
	c.mu.Unlock()

	if !connected || stream == nil {
		c.logger.Warn().Str("request_id", result.RequestId).Msg("not connected, dropping health check result")
		return
	}

	msg := NewHealthCheckResultMessage(result)
	if err := stream.Send(msg); err != nil {
		c.logger.Warn().Err(err).Str("request_id", result.RequestId).Msg("failed to send health check result")
	}
}
```

- [ ] **Step 3: Add SendHealthCheckResult to GRPCClient interface**

Add `SendHealthCheckResult(result *pb.HealthCheckResult)` to the `GRPCClient` interface in `internal/app/interfaces.go` (after `SendExecResult`).

- [ ] **Step 4: Verify build**

Run:
```bash
go build ./...
```
Expected: clean build

- [ ] **Step 5: Commit**

```bash
git add internal/grpcclient/sender.go internal/grpcclient/client.go internal/app/interfaces.go
git commit -m "feat(checker): add SendHealthCheckResult to gRPC sender and interface"
```

---

## Task 8: Agent Wiring

**Files:**
- Modify: `internal/app/agent.go`

- [ ] **Step 1: Add checker blank imports**

Add after the existing blank imports (line 44) in `agent.go`:

```go
_ "github.com/cy77cc/opsagent/internal/checker/kernel"
_ "github.com/cy77cc/opsagent/internal/checker/filesystem"
_ "github.com/cy77cc/opsagent/internal/checker/network"
_ "github.com/cy77cc/opsagent/internal/checker/service"
_ "github.com/cy77cc/opsagent/internal/checker/container"
```

- [ ] **Step 2: Add checker executor to Agent struct**

Add field to `Agent` struct (after `sandboxExec`):

```go
checkerExec *checker.Executor
```

Add import for `"github.com/cy77cc/opsagent/internal/checker"`.

- [ ] **Step 3: Build checker executor in NewAgent()**

Add after the sandbox executor block (line 222) in `NewAgent()`:

```go
// Build checker executor if enabled.
if cfg.Checker.Enabled {
    a.checkerExec = checker.NewExecutor(checker.DefaultRegistry, log)
}
```

- [ ] **Step 4: Register health check gRPC handler**

Add in `registerGRPCHandlers()` after the config update handler (line 994):

```go
// Health check handler: execute system health checks.
recv.SetHealthCheckHandler(func(ctx context.Context, req *pb.HealthCheckRequest) error {
    if a.shuttingDown.Load() {
        return fmt.Errorf("agent is shutting down")
    }
    if a.checkerExec == nil {
        a.log.Warn().Str("request_id", req.RequestId).Msg("checker not enabled, skipping health check")
        return nil
    }

    a.auditLog.Log(AuditEvent{
        EventType: "health_check.started", Component: "checker",
        Action: "health_check", Status: "success",
        Details: map[string]interface{}{"request_id": req.RequestId, "item_count": len(req.Items)},
    })

    start := time.Now()
    err := a.checkerExec.Execute(ctx, req, func(result *pb.HealthCheckResult) {
        a.grpcClient.SendHealthCheckResult(result)
    })

    a.auditLog.Log(AuditEvent{
        EventType: "health_check.completed", Component: "checker",
        Action: "health_check", Status: "success",
        Details: map[string]interface{}{
            "request_id": req.RequestId,
            "duration_ms": time.Since(start).Milliseconds(),
        },
    })

    return err
})
```

- [ ] **Step 5: Add checker capabilities to registration**

In `NewAgent()`, after building the gRPC client config, add checker types to capabilities:

```go
caps := []string{"health_check"}
for _, typ := range checker.DefaultRegistry.Types() {
    caps = append(caps, "checker:"+typ)
}
grpcCfg.Capabilities = caps
```

- [ ] **Step 6: Verify build**

Run:
```bash
go build ./...
```
Expected: clean build (blank imports will fail until checker packages exist — implement Tasks 9-13 first, or use stub packages)

- [ ] **Step 7: Commit**

```bash
git add internal/app/agent.go
git commit -m "feat(checker): wire checker executor and gRPC handler in agent"
```

---

## Task 9: Kernel Checkers

**Files:**
- Create: `internal/checker/kernel/sysctl.go`
- Create: `internal/checker/kernel/kernel_version.go`
- Create: `internal/checker/kernel/kernel_module.go`
- Create: `internal/checker/kernel/boot_param.go`
- Create: `internal/checker/kernel/kernel_test.go`

- [ ] **Step 1: Write kernel_test.go**

```go
package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSysctlCheckPass(t *testing.T) {
	c := &SysctlChecker{}
	assert.Equal(t, "sysctl_check", c.Type())
	assert.Equal(t, "kernel", c.Category())

	params := json.RawMessage(`{"path": "/proc/sys/kernel/hostname", "expected": ""}`)
	// hostname is always non-empty, so this tests the read path
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ActualValue)
}

func TestSysctlCheckMissingParams(t *testing.T) {
	c := &SysctlChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
}

func TestSysctlCheckPathTraversal(t *testing.T) {
	c := &SysctlChecker{}
	params := json.RawMessage(`{"path": "/proc/sys/../../../etc/passwd", "expected": "x"}`)
	_, err := c.Check(context.Background(), params)
	assert.Error(t, err)
}

func TestKernelVersionCheck(t *testing.T) {
	c := &KernelVersionChecker{}
	assert.Equal(t, "kernel_version_check", c.Type())

	result, err := c.Check(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.NotEmpty(t, result.ActualValue)
	assert.Equal(t, checker.StatusPass, result.Status)
}

func TestKernelModuleCheck(t *testing.T) {
	c := &KernelModuleChecker{}
	assert.Equal(t, "kernel_module_check", c.Type())

	// Check for a module that is likely loaded
	params := json.RawMessage(`{"module": "ext4", "expected": "loaded"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Result depends on system, just verify no error
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail, checker.StatusWarn}, result.Status)
}

func TestBootParamCheck(t *testing.T) {
	c := &BootParamChecker{}
	assert.Equal(t, "boot_param_check", c.Type())

	result, err := c.Check(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.NotEmpty(t, result.ActualValue)
}
```

- [ ] **Step 2: Write sysctl.go**

```go
package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&SysctlChecker{})
}

// SysctlChecker reads a sysctl parameter from /proc/sys/ and compares to expected.
type SysctlChecker struct{}

func (c *SysctlChecker) Type() string     { return "sysctl_check" }
func (c *SysctlChecker) Category() string { return "kernel" }

type sysctlParams struct {
	Path     string `json:"path"`     // e.g., "/proc/sys/net/ipv4/ip_forward"
	Expected string `json:"expected"` // expected value
}

func (c *SysctlChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p sysctlParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("sysctl_check: invalid params: %w", err)
	}
	if p.Path == "" {
		return nil, fmt.Errorf("sysctl_check: path is required")
	}

	// Path traversal prevention: must be under /proc/sys/
	cleaned := filepath.Clean(p.Path)
	if !strings.HasPrefix(cleaned, "/proc/sys/") {
		return nil, fmt.Errorf("sysctl_check: path must be under /proc/sys/, got %s", cleaned)
	}

	data, err := os.ReadFile(cleaned)
	if err != nil {
		return nil, fmt.Errorf("sysctl_check: read %s: %w", cleaned, err)
	}
	actual := strings.TrimSpace(string(data))

	status := checker.StatusPass
	message := "parameter matches expected value"
	if actual != p.Expected {
		status = checker.StatusFail
		message = fmt.Sprintf("parameter mismatch: expected %q, got %q", p.Expected, actual)
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       message,
	}, nil
}
```

- [ ] **Step 3: Write kernel_version.go**

```go
package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&KernelVersionChecker{})
}

// KernelVersionChecker reports the running kernel version.
type KernelVersionChecker struct{}

func (c *KernelVersionChecker) Type() string     { return "kernel_version_check" }
func (c *KernelVersionChecker) Category() string { return "kernel" }

func (c *KernelVersionChecker) Check(_ context.Context, _ json.RawMessage) (*checker.CheckResult, error) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return nil, fmt.Errorf("kernel_version_check: uname failed: %w", err)
	}

	// Convert [65]int8 to string
	release := charsToString(uname.Release[:])

	return &checker.CheckResult{
		Status:      checker.StatusPass,
		ActualValue: release,
		Message:     fmt.Sprintf("kernel version: %s", release),
	}, nil
}

func charsToString(ca []int8) string {
	s := make([]byte, 0, len(ca))
	for _, c := range ca {
		if c == 0 {
			break
		}
		s = append(s, byte(c))
	}
	return string(s)
}
```

- [ ] **Step 4: Write kernel_module.go**

```go
package kernel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&KernelModuleChecker{})
}

// KernelModuleChecker checks whether a kernel module is loaded.
type KernelModuleChecker struct{}

func (c *KernelModuleChecker) Type() string     { return "kernel_module_check" }
func (c *KernelModuleChecker) Category() string { return "kernel" }

type moduleParams struct {
	Module   string `json:"module"`   // module name
	Expected string `json:"expected"` // "loaded" or "not_loaded"
}

func (c *KernelModuleChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p moduleParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("kernel_module_check: invalid params: %w", err)
	}
	if p.Module == "" {
		return nil, fmt.Errorf("kernel_module_check: module is required")
	}

	loaded, err := isModuleLoaded(p.Module)
	if err != nil {
		return nil, err
	}

	actual := "not_loaded"
	if loaded {
		actual = "loaded"
	}

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("module %s: %s", p.Module, actual),
	}, nil
}

func isModuleLoaded(name string) (bool, error) {
	f, err := os.Open("/proc/modules")
	if err != nil {
		return false, fmt.Errorf("kernel_module_check: open /proc/modules: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) > 0 && parts[0] == name {
			return true, nil
		}
	}
	return false, scanner.Err()
}
```

- [ ] **Step 5: Write boot_param.go**

```go
package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&BootParamChecker{})
}

// BootParamChecker checks kernel boot parameters from /proc/cmdline.
type BootParamChecker struct{}

func (c *BootParamChecker) Type() string     { return "boot_param_check" }
func (c *BootParamChecker) Category() string { return "kernel" }

type bootParamParams struct {
	Param    string `json:"param"`    // e.g., "selinux"
	Expected string `json:"expected"` // e.g., "1"
}

func (c *BootParamChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p bootParamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("boot_param_check: invalid params: %w", err)
	}
	if p.Param == "" {
		return nil, fmt.Errorf("boot_param_check: param is required")
	}

	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return nil, fmt.Errorf("boot_param_check: read /proc/cmdline: %w", err)
	}
	cmdline := strings.TrimSpace(string(data))

	// Find the param value
	actual := findBootParam(cmdline, p.Param)

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("boot param %s=%s", p.Param, actual),
	}, nil
}

func findBootParam(cmdline, param string) string {
	for _, part := range strings.Fields(cmdline) {
		if strings.HasPrefix(part, param+"=") {
			return strings.TrimPrefix(part, param+"=")
		}
		if part == param {
			return "true"
		}
	}
	return ""
}
```

- [ ] **Step 6: Run tests**

Run:
```bash
go test ./internal/checker/kernel/ -v -count=1
```
Expected: PASS (some tests may vary by system)

- [ ] **Step 7: Commit**

```bash
git add internal/checker/kernel/
git commit -m "feat(checker): add kernel checkers (sysctl, version, module, boot_param)"
```

---

## Task 10: Filesystem Checkers

**Files:**
- Create: `internal/checker/filesystem/file_perm.go`
- Create: `internal/checker/filesystem/file_exist.go`
- Create: `internal/checker/filesystem/dir_perm.go`
- Create: `internal/checker/filesystem/mount_option.go`
- Create: `internal/checker/filesystem/filesystem_test.go`

- [ ] **Step 1: Write filesystem_test.go**

```go
package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePermCheck(t *testing.T) {
	// Create a temp file with known permissions
	dir := t.TempDir()
	f := filepath.Join(dir, "testfile")
	require.NoError(t, os.WriteFile(f, []byte("test"), 0644))

	c := &FilePermChecker{}
	params := json.RawMessage(`{"path": "` + f + `", "expected_mode": "0644", "expected_owner": "root"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Mode should match, owner check depends on who runs the test
	assert.Equal(t, "0644", result.ActualValue)
}

func TestFilePermCheckPathTraversal(t *testing.T) {
	c := &FilePermChecker{}
	params := json.RawMessage(`{"path": "/tmp/../../../etc/shadow", "expected_mode": "0640"}`)
	_, err := c.Check(context.Background(), params)
	assert.Error(t, err)
}

func TestFileExistCheck(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "exists")
	require.NoError(t, os.WriteFile(f, []byte(""), 0644))

	c := &FileExistChecker{}
	params := json.RawMessage(`{"path": "` + f + `", "expected": "exists"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
}

func TestFileExistCheckMissing(t *testing.T) {
	c := &FileExistChecker{}
	params := json.RawMessage(`{"path": "/tmp/nonexistent_file_xyz", "expected": "exists"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
}

func TestDirPermCheck(t *testing.T) {
	dir := t.TempDir()
	c := &DirPermChecker{}
	params := json.RawMessage(`{"path": "` + dir + `", "expected_mode": "0755"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ActualValue)
}

func TestMountOptionCheck(t *testing.T) {
	c := &MountOptionChecker{}
	assert.Equal(t, "mount_option_check", c.Type())
	// Just verify it doesn't crash reading /proc/mounts
	params := json.RawMessage(`{"mount_point": "/", "expected_option": "rw"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
```

- [ ] **Step 2: Write file_perm.go**

```go
package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&FilePermChecker{})
}

type FilePermChecker struct{}

func (c *FilePermChecker) Type() string     { return "file_perm_check" }
func (c *FilePermChecker) Category() string { return "filesystem" }

type filePermParams struct {
	Path         string `json:"path"`
	ExpectedMode string `json:"expected_mode"` // octal string like "0644"
	ExpectedOwner string `json:"expected_owner,omitempty"`
}

func (c *FilePermChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p filePermParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("file_perm_check: invalid params: %w", err)
	}
	if p.Path == "" {
		return nil, fmt.Errorf("file_perm_check: path is required")
	}

	cleaned := filepath.Clean(p.Path)
	info, err := os.Lstat(cleaned)
	if err != nil {
		return nil, fmt.Errorf("file_perm_check: stat %s: %w", cleaned, err)
	}

	actualMode := fmt.Sprintf("0%o", info.Mode().Perm())
	expectedMode := p.ExpectedMode

	status := checker.StatusPass
	message := "file permissions match"

	if actualMode != expectedMode {
		status = checker.StatusFail
		message = fmt.Sprintf("mode mismatch: expected %s, got %s", expectedMode, actualMode)
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actualMode,
		ExpectedValue: expectedMode,
		Message:       message,
	}, nil
}
```

- [ ] **Step 3: Write file_exist.go**

```go
package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&FileExistChecker{})
}

type FileExistChecker struct{}

func (c *FileExistChecker) Type() string     { return "file_exist_check" }
func (c *FileExistChecker) Category() string { return "filesystem" }

type fileExistParams struct {
	Path     string `json:"path"`
	Expected string `json:"expected"` // "exists" or "not_exists"
}

func (c *FileExistChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p fileExistParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("file_exist_check: invalid params: %w", err)
	}
	if p.Path == "" {
		return nil, fmt.Errorf("file_exist_check: path is required")
	}

	cleaned := filepath.Clean(p.Path)
	_, err := os.Stat(cleaned)
	actual := "exists"
	if os.IsNotExist(err) {
		actual = "not_exists"
	} else if err != nil {
		return nil, fmt.Errorf("file_exist_check: stat %s: %w", cleaned, err)
	}

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("file %s: %s", cleaned, actual),
	}, nil
}
```

- [ ] **Step 4: Write dir_perm.go**

```go
package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&DirPermChecker{})
}

type DirPermChecker struct{}

func (c *DirPermChecker) Type() string     { return "dir_perm_check" }
func (c *DirPermChecker) Category() string { return "filesystem" }

type dirPermParams struct {
	Path         string `json:"path"`
	ExpectedMode string `json:"expected_mode"`
	StickyBit    *bool  `json:"sticky_bit,omitempty"` // nil = don't check
}

func (c *DirPermChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p dirPermParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("dir_perm_check: invalid params: %w", err)
	}
	if p.Path == "" {
		return nil, fmt.Errorf("dir_perm_check: path is required")
	}

	cleaned := filepath.Clean(p.Path)
	info, err := os.Stat(cleaned)
	if err != nil {
		return nil, fmt.Errorf("dir_perm_check: stat %s: %w", cleaned, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("dir_perm_check: %s is not a directory", cleaned)
	}

	actualMode := fmt.Sprintf("0%o", info.Mode().Perm())
	status := checker.StatusPass
	message := "directory permissions match"

	if actualMode != p.ExpectedMode {
		status = checker.StatusFail
		message = fmt.Sprintf("mode mismatch: expected %s, got %s", p.ExpectedMode, actualMode)
	}

	if p.StickyBit != nil {
		hasSticky := info.Mode()&os.ModeSticky != 0
		if hasSticky != *p.StickyBit {
			status = checker.StatusFail
			message = fmt.Sprintf("sticky bit: expected %v, got %v", *p.StickyBit, hasSticky)
		}
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actualMode,
		ExpectedValue: p.ExpectedMode,
		Message:       message,
	}, nil
}
```

- [ ] **Step 5: Write mount_option.go**

```go
package filesystem

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&MountOptionChecker{})
}

type MountOptionChecker struct{}

func (c *MountOptionChecker) Type() string     { return "mount_option_check" }
func (c *MountOptionChecker) Category() string { return "filesystem" }

type mountOptionParams struct {
	MountPoint     string `json:"mount_point"`
	ExpectedOption string `json:"expected_option"` // e.g., "noexec", "nosuid", "nodev"
}

func (c *MountOptionChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p mountOptionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("mount_option_check: invalid params: %w", err)
	}
	if p.MountPoint == "" {
		return nil, fmt.Errorf("mount_option_check: mount_point is required")
	}

	options, err := getMountOptions(p.MountPoint)
	if err != nil {
		return nil, err
	}

	found := false
	for _, opt := range options {
		if opt == p.ExpectedOption {
			found = true
			break
		}
	}

	actual := "not_present"
	if found {
		actual = "present"
	}

	status := checker.StatusPass
	if !found {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: "present",
		Message:       fmt.Sprintf("mount option %s on %s: %s", p.ExpectedOption, p.MountPoint, actual),
	}, nil
}

func getMountOptions(mountPoint string) ([]string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("mount_option_check: open /proc/mounts: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 4 && parts[1] == mountPoint {
			return strings.Split(parts[3], ","), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("mount_option_check: read /proc/mounts: %w", err)
	}
	return nil, fmt.Errorf("mount_option_check: mount point %s not found", mountPoint)
}
```

- [ ] **Step 6: Run tests**

Run:
```bash
go test ./internal/checker/filesystem/ -v -count=1
```
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/checker/filesystem/
git commit -m "feat(checker): add filesystem checkers (file_perm, file_exist, dir_perm, mount_option)"
```

---

## Task 11: Network Checkers

**Files:**
- Create: `internal/checker/network/port.go`
- Create: `internal/checker/network/ssh_config.go`
- Create: `internal/checker/network/iptables.go`
- Create: `internal/checker/network/network_param.go`
- Create: `internal/checker/network/network_test.go`

- [ ] **Step 1: Write network_test.go**

```go
package network

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortCheck(t *testing.T) {
	c := &PortChecker{}
	assert.Equal(t, "port_check", c.Type())
	assert.Equal(t, "network", c.Category())

	// Check a port that's almost certainly not listening
	params := json.RawMessage(`{"port": 59999, "expected_state": "not_listening"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
}

func TestPortCheckInvalidParams(t *testing.T) {
	c := &PortChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
}

func TestSSHConfigCheck(t *testing.T) {
	c := &SSHConfigChecker{}
	assert.Equal(t, "ssh_config_check", c.Type())

	params := json.RawMessage(`{"key": "PermitRootLogin", "expected": "no"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestIptablesCheck(t *testing.T) {
	c := &IptablesChecker{}
	assert.Equal(t, "iptables_check", c.Type())

	params := json.RawMessage(`{"chain": "INPUT", "expected_policy": "DROP"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNetworkParamCheck(t *testing.T) {
	c := &NetworkParamChecker{}
	assert.Equal(t, "network_param_check", c.Type())

	params := json.RawMessage(`{"key": "net.ipv4.ip_forward", "expected": "0"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
```

- [ ] **Step 2: Write port.go**

```go
package network

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&PortChecker{})
}

type PortChecker struct{}

func (c *PortChecker) Type() string     { return "port_check" }
func (c *PortChecker) Category() string { return "network" }

type portParams struct {
	Port         int    `json:"port"`
	ExpectedState string `json:"expected_state"` // "listening" or "not_listening"
}

func (c *PortChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p portParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("port_check: invalid params: %w", err)
	}
	if p.Port <= 0 || p.Port > 65535 {
		return nil, fmt.Errorf("port_check: port must be 1-65535")
	}

	listening, err := isPortListening(p.Port)
	if err != nil {
		return nil, err
	}

	actual := "not_listening"
	if listening {
		actual = "listening"
	}

	status := checker.StatusPass
	if actual != p.ExpectedState {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.ExpectedState,
		Message:       fmt.Sprintf("port %d: %s", p.Port, actual),
	}, nil
}

func isPortListening(port int) (bool, error) {
	hexPort := fmt.Sprintf("%04X", port)

	for _, proto := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(proto)
		if err != nil {
			continue
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}
			// State "0A" = LISTEN
			if fields[3] != "0A" {
				continue
			}
			// Local address is ip:port in hex
			parts := strings.Split(fields[1], ":")
			if len(parts) == 2 && parts[1] == hexPort {
				return true, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("port_check: read %s: %w", proto, err)
		}
	}
	return false, nil
}
```

- [ ] **Step 3: Write ssh_config.go**

```go
package network

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&SSHConfigChecker{})
}

type SSHConfigChecker struct{}

func (c *SSHConfigChecker) Type() string     { return "ssh_config_check" }
func (c *SSHConfigChecker) Category() string { return "network" }

type sshConfigParams struct {
	Key      string `json:"key"`      // e.g., "PermitRootLogin"
	Expected string `json:"expected"` // e.g., "no"
}

func (c *SSHConfigChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p sshConfigParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("ssh_config_check: invalid params: %w", err)
	}
	if p.Key == "" {
		return nil, fmt.Errorf("ssh_config_check: key is required")
	}

	actual, err := getSSHConfigValue(p.Key)
	if err != nil {
		return nil, err
	}

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("sshd %s = %s", p.Key, actual),
	}, nil
}

func getSSHConfigValue(key string) (string, error) {
	f, err := os.Open("/etc/ssh/sshd_config")
	if err != nil {
		return "", fmt.Errorf("ssh_config_check: open sshd_config: %w", err)
	}
	defer f.Close()

	lowerKey := strings.ToLower(key)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == lowerKey {
			return strings.TrimSpace(parts[1]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("ssh_config_check: read sshd_config: %w", err)
	}
	return "", fmt.Errorf("ssh_config_check: key %q not found in sshd_config", key)
}
```

- [ ] **Step 4: Write iptables.go**

```go
package network

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&IptablesChecker{})
}

type IptablesChecker struct{}

func (c *IptablesChecker) Type() string     { return "iptables_check" }
func (c *IptablesChecker) Category() string { return "network" }

type iptablesParams struct {
	Chain          string `json:"chain"`           // e.g., "INPUT"
	ExpectedPolicy string `json:"expected_policy"` // e.g., "DROP", "ACCEPT"
}

func (c *IptablesChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p iptablesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("iptables_check: invalid params: %w", err)
	}
	if p.Chain == "" {
		return nil, fmt.Errorf("iptables_check: chain is required")
	}

	policy, err := getIptablesPolicy(p.Chain)
	if err != nil {
		return nil, err
	}

	status := checker.StatusPass
	if policy != p.ExpectedPolicy {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   policy,
		ExpectedValue: p.ExpectedPolicy,
		Message:       fmt.Sprintf("iptables chain %s policy: %s", p.Chain, policy),
	}, nil
}

func getIptablesPolicy(chain string) (string, error) {
	out, err := exec.Command("iptables", "-L", chain, "-n").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("iptables_check: iptables command failed: %w: %s", err, string(out))
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		// First line: "Chain INPUT (policy DROP)"
		if strings.HasPrefix(line, "Chain "+chain) {
			start := strings.Index(line, "(policy ")
			if start >= 0 {
				end := strings.Index(line[start:], ")")
				if end > 0 {
					return strings.TrimSpace(line[start+8 : start+end]), nil
				}
			}
		}
	}
	return "", fmt.Errorf("iptables_check: could not parse policy for chain %s", chain)
}
```

- [ ] **Step 5: Write network_param.go**

```go
package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&NetworkParamChecker{})
}

type NetworkParamChecker struct{}

func (c *NetworkParamChecker) Type() string     { return "network_param_check" }
func (c *NetworkParamChecker) Category() string { return "network" }

type networkParamParams struct {
	Key      string `json:"key"`      // e.g., "net.ipv4.ip_forward"
	Expected string `json:"expected"` // expected value
}

func (c *NetworkParamChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p networkParamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("network_param_check: invalid params: %w", err)
	}
	if p.Key == "" {
		return nil, fmt.Errorf("network_param_check: key is required")
	}

	// Convert sysctl key to /proc/sys path
	procPath := "/proc/sys/" + strings.ReplaceAll(p.Key, ".", "/")
	cleaned := filepath.Clean(procPath)
	if !strings.HasPrefix(cleaned, "/proc/sys/") {
		return nil, fmt.Errorf("network_param_check: invalid key %s", p.Key)
	}

	data, err := os.ReadFile(cleaned)
	if err != nil {
		return nil, fmt.Errorf("network_param_check: read %s: %w", cleaned, err)
	}
	actual := strings.TrimSpace(string(data))

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("%s = %s", p.Key, actual),
	}, nil
}
```

- [ ] **Step 6: Run tests**

Run:
```bash
go test ./internal/checker/network/ -v -count=1
```
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/checker/network/
git commit -m "feat(checker): add network checkers (port, ssh_config, iptables, network_param)"
```

---

## Task 12: Service Checkers

**Files:**
- Create: `internal/checker/service/service.go`
- Create: `internal/checker/service/user.go`
- Create: `internal/checker/service/cron.go`
- Create: `internal/checker/service/pam.go`
- Create: `internal/checker/service/service_test.go`

- [ ] **Step 1: Write service_test.go**

```go
package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceCheck(t *testing.T) {
	c := &ServiceChecker{}
	assert.Equal(t, "service_check", c.Type())
	assert.Equal(t, "service", c.Category())

	params := json.RawMessage(`{"name": "nonexistent_service_xyz", "expected_status": "inactive"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
}

func TestServiceCheckMissingParams(t *testing.T) {
	c := &ServiceChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{}`))
	assert.Error(t, err)
}

func TestUserCheck(t *testing.T) {
	c := &UserChecker{}
	assert.Equal(t, "user_check", c.Type())

	params := json.RawMessage(`{"username": "root", "check": "exists"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
}

func TestUserCheckNonexistent(t *testing.T) {
	c := &UserChecker{}
	params := json.RawMessage(`{"username": "nonexistent_user_xyz", "check": "exists"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
}

func TestCronCheck(t *testing.T) {
	c := &CronChecker{}
	assert.Equal(t, "cron_check", c.Type())

	params := json.RawMessage(`{"user": "root"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestPamCheck(t *testing.T) {
	c := &PamChecker{}
	assert.Equal(t, "pam_check", c.Type())

	params := json.RawMessage(`{"module": "pam_unix.so", "file": "common-auth"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
```

- [ ] **Step 2: Write service.go**

```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&ServiceChecker{})
}

type ServiceChecker struct{}

func (c *ServiceChecker) Type() string     { return "service_check" }
func (c *ServiceChecker) Category() string { return "service" }

type serviceParams struct {
	Name           string `json:"name"`
	ExpectedStatus string `json:"expected_status"` // "active", "inactive", "failed"
}

func (c *ServiceChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p serviceParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("service_check: invalid params: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("service_check: name is required")
	}

	out, err := exec.Command("systemctl", "is-active", p.Name).CombinedOutput()
	actual := strings.TrimSpace(string(out))

	// systemctl is-active exits 0 for "active", non-zero for others
	if err != nil {
		// Not an error — just means the service isn't active
		if actual == "" {
			actual = "inactive"
		}
	}

	status := checker.StatusPass
	if actual != p.ExpectedStatus {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.ExpectedStatus,
		Message:       fmt.Sprintf("service %s: %s", p.Name, actual),
	}, nil
}
```

- [ ] **Step 3: Write user.go**

```go
package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&UserChecker{})
}

type UserChecker struct{}

func (c *UserChecker) Type() string     { return "user_check" }
func (c *UserChecker) Category() string { return "service" }

type userParams struct {
	Username string `json:"username"`
	Check    string `json:"check"` // "exists", "locked", "shell"
}

func (c *UserChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p userParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("user_check: invalid params: %w", err)
	}
	if p.Username == "" {
		return nil, fmt.Errorf("user_check: username is required")
	}

	switch p.Check {
	case "exists":
		return c.checkExists(p.Username)
	case "locked":
		return c.checkLocked(p.Username)
	default:
		return nil, fmt.Errorf("user_check: unknown check type %q", p.Check)
	}
}

func (c *UserChecker) checkExists(username string) (*checker.CheckResult, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, fmt.Errorf("user_check: open /etc/passwd: %w", err)
	}
	defer f.Close()

	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) > 0 && parts[0] == username {
			found = true
			break
		}
	}

	actual := "not_exists"
	if found {
		actual = "exists"
	}

	status := checker.StatusPass
	if !found {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: "exists",
		Message:       fmt.Sprintf("user %s: %s", username, actual),
	}, nil
}

func (c *UserChecker) checkLocked(username string) (*checker.CheckResult, error) {
	f, err := os.Open("/etc/shadow")
	if err != nil {
		return nil, fmt.Errorf("user_check: open /etc/shadow: %w", err)
	}
	defer f.Close()

	locked := false
	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 2 && parts[0] == username {
			found = true
			// Locked accounts have "!" or "!!" prefix in password field
			locked = strings.HasPrefix(parts[1], "!") || parts[1] == "*"
			break
		}
	}

	if !found {
		return &checker.CheckResult{
			Status:  checker.StatusError,
			Message: fmt.Sprintf("user %s not found in /etc/shadow", username),
		}, nil
	}

	actual := "unlocked"
	if locked {
		actual = "locked"
	}

	return &checker.CheckResult{
		Status:      checker.StatusPass,
		ActualValue: actual,
		Message:     fmt.Sprintf("user %s: %s", username, actual),
	}, nil
}
```

- [ ] **Step 4: Write cron.go**

```go
package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&CronChecker{})
}

type CronChecker struct{}

func (c *CronChecker) Type() string     { return "cron_check" }
func (c *CronChecker) Category() string { return "service" }

type cronParams struct {
	User string `json:"user"` // user whose crontab to check
}

func (c *CronChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p cronParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("cron_check: invalid params: %w", err)
	}
	if p.User == "" {
		return nil, fmt.Errorf("cron_check: user is required")
	}

	entries, err := getCronEntries(p.User)
	if err != nil {
		return nil, err
	}

	return &checker.CheckResult{
		Status:    checker.StatusPass,
		ActualValue: fmt.Sprintf("%d entries", len(entries)),
		Message:   fmt.Sprintf("user %s has %d cron entries", p.User, len(entries)),
	}, nil
}

func getCronEntries(user string) ([]string, error) {
	cronPath := filepath.Join("/var/spool/cron/crontabs", user)
	f, err := os.Open(cronPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cron_check: open %s: %w", cronPath, err)
	}
	defer f.Close()

	var entries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, line)
		}
	}
	return entries, scanner.Err()
}
```

- [ ] **Step 5: Write pam.go**

```go
package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&PamChecker{})
}

type PamChecker struct{}

func (c *PamChecker) Type() string     { return "pam_check" }
func (c *PamChecker) Category() string { return "service" }

type pamParams struct {
	Module string `json:"module"` // e.g., "pam_unix.so"
	File   string `json:"file"`   // e.g., "common-auth"
}

func (c *PamChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p pamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("pam_check: invalid params: %w", err)
	}
	if p.Module == "" || p.File == "" {
		return nil, fmt.Errorf("pam_check: module and file are required")
	}

	pamPath := filepath.Join("/etc/pam.d", p.File)
	f, err := os.Open(pamPath)
	if err != nil {
		return nil, fmt.Errorf("pam_check: open %s: %w", pamPath, err)
	}
	defer f.Close()

	found := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, p.Module) {
			found = true
			break
		}
	}

	actual := "not_found"
	if found {
		actual = "found"
	}

	status := checker.StatusPass
	if !found {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: "found",
		Message:       fmt.Sprintf("PAM module %s in %s: %s", p.Module, p.File, actual),
	}, nil
}
```

- [ ] **Step 6: Run tests**

Run:
```bash
go test ./internal/checker/service/ -v -count=1
```
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/checker/service/
git commit -m "feat(checker): add service checkers (service, user, cron, pam)"
```

---

## Task 13: Container Checkers

**Files:**
- Create: `internal/checker/container/docker.go`
- Create: `internal/checker/container/containerd.go`
- Create: `internal/checker/container/cgroup.go`
- Create: `internal/checker/container/runtime.go`
- Create: `internal/checker/container/container_test.go`

- [ ] **Step 1: Write container_test.go**

```go
package container

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerCheck(t *testing.T) {
	c := &DockerChecker{}
	assert.Equal(t, "docker_check", c.Type())
	assert.Equal(t, "container", c.Category())

	params := json.RawMessage(`{"check": "daemon_json", "key": "storage-driver"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestContainerdCheck(t *testing.T) {
	c := &ContainerdChecker{}
	assert.Equal(t, "containerd_check", c.Type())

	params := json.RawMessage(`{"key": "version"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestCgroupCheck(t *testing.T) {
	c := &CgroupChecker{}
	assert.Equal(t, "cgroup_check", c.Type())

	result, err := c.Check(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.ActualValue)
}

func TestContainerRuntimeCheck(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	assert.Equal(t, "container_runtime_check", c.Type())

	params := json.RawMessage(`{"runtime": "docker", "expected": "available"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.NotNil(t, result)
}
```

- [ ] **Step 2: Write docker.go**

```go
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&DockerChecker{})
}

type DockerChecker struct{}

func (c *DockerChecker) Type() string     { return "docker_check" }
func (c *DockerChecker) Category() string { return "container" }

type dockerParams struct {
	Check    string `json:"check"`     // "daemon_json"
	Key      string `json:"key"`       // key to check in daemon.json
	Expected string `json:"expected"`  // expected value
}

func (c *DockerChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p dockerParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("docker_check: invalid params: %w", err)
	}

	switch p.Check {
	case "daemon_json":
		return c.checkDaemonJSON(p.Key, p.Expected)
	default:
		return nil, fmt.Errorf("docker_check: unknown check type %q", p.Check)
	}
}

func (c *DockerChecker) checkDaemonJSON(key, expected string) (*checker.CheckResult, error) {
	data, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil {
		if os.IsNotExist(err) {
			return &checker.CheckResult{
				Status:  checker.StatusWarn,
				Message: "/etc/docker/daemon.json not found",
			}, nil
		}
		return nil, fmt.Errorf("docker_check: read daemon.json: %w", err)
	}

	// Simple string search for the key (avoids importing encoding/json for map parsing)
	content := string(data)
	searchKey := fmt.Sprintf(`"%s"`, key)
	if !strings.Contains(content, searchKey) {
		return &checker.CheckResult{
			Status:        checker.StatusFail,
			ActualValue:   "not_found",
			ExpectedValue: expected,
			Message:       fmt.Sprintf("key %q not found in daemon.json", key),
		}, nil
	}

	return &checker.CheckResult{
		Status:  checker.StatusPass,
		Message: fmt.Sprintf("key %q found in daemon.json", key),
	}, nil
}
```

- [ ] **Step 3: Write containerd.go**

```go
package container

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&ContainerdChecker{})
}

type ContainerdChecker struct{}

func (c *ContainerdChecker) Type() string     { return "containerd_check" }
func (c *ContainerdChecker) Category() string { return "container" }

type containerdParams struct {
	Key      string `json:"key"`
	Expected string `json:"expected"`
}

func (c *ContainerdChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p containerdParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("containerd_check: invalid params: %w", err)
	}
	if p.Key == "" {
		return nil, fmt.Errorf("containerd_check: key is required")
	}

	f, err := os.Open("/etc/containerd/config.toml")
	if err != nil {
		if os.IsNotExist(err) {
			return &checker.CheckResult{
				Status:  checker.StatusSkip,
				Message: "/etc/containerd/config.toml not found, containerd may not be installed",
			}, nil
		}
		return nil, fmt.Errorf("containerd_check: open config.toml: %w", err)
	}
	defer f.Close()

	found := false
	var actual string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, p.Key) {
			found = true
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				actual = strings.TrimSpace(parts[1])
				actual = strings.Trim(actual, `"`)
			}
			break
		}
	}

	if !found {
		return &checker.CheckResult{
			Status:        checker.StatusFail,
			ActualValue:   "not_found",
			ExpectedValue: p.Expected,
			Message:       fmt.Sprintf("key %q not found in containerd config", p.Key),
		}, nil
	}

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("containerd %s = %s", p.Key, actual),
	}, nil
}
```

- [ ] **Step 4: Write cgroup.go**

```go
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&CgroupChecker{})
}

type CgroupChecker struct{}

func (c *CgroupChecker) Type() string     { return "cgroup_check" }
func (c *CgroupChecker) Category() string { return "container" }

func (c *CgroupChecker) Check(_ context.Context, _ json.RawMessage) (*checker.CheckResult, error) {
	// Detect cgroup version
	v1, err := os.Stat("/sys/fs/cgroup/cpu")
	v2, err2 := os.Stat("/sys/fs/cgroup/cgroup.controllers")

	version := "unknown"
	if err2 == nil && v2 != nil {
		version = "v2"
	} else if err == nil && v1 != nil {
		version = "v1"
	}

	return &checker.CheckResult{
		Status:      checker.StatusPass,
		ActualValue: version,
		Message:     fmt.Sprintf("cgroup version: %s", version),
	}, nil
}
```

- [ ] **Step 5: Write runtime.go**

```go
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&ContainerRuntimeChecker{})
}

type ContainerRuntimeChecker struct{}

func (c *ContainerRuntimeChecker) Type() string     { return "container_runtime_check" }
func (c *ContainerRuntimeChecker) Category() string { return "container" }

type runtimeParams struct {
	Runtime  string `json:"runtime"`  // "docker", "containerd", "cri-o"
	Expected string `json:"expected"` // "available" or "unavailable"
}

func (c *ContainerRuntimeChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p runtimeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("container_runtime_check: invalid params: %w", err)
	}
	if p.Runtime == "" {
		return nil, fmt.Errorf("container_runtime_check: runtime is required")
	}

	sockets := map[string]string{
		"docker":     "/var/run/docker.sock",
		"containerd": "/var/run/containerd/containerd.sock",
		"cri-o":      "/var/run/crio/crio.sock",
	}

	sockPath, ok := sockets[p.Runtime]
	if !ok {
		return nil, fmt.Errorf("container_runtime_check: unknown runtime %q", p.Runtime)
	}

	_, err := os.Stat(sockPath)
	actual := "unavailable"
	if err == nil {
		actual = "available"
	}

	status := checker.StatusPass
	if actual != p.Expected {
		status = checker.StatusFail
	}

	return &checker.CheckResult{
		Status:        status,
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Message:       fmt.Sprintf("container runtime %s: %s (socket: %s)", p.Runtime, actual, sockPath),
	}, nil
}
```

- [ ] **Step 6: Run tests**

Run:
```bash
go test ./internal/checker/container/ -v -count=1
```
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/checker/container/
git commit -m "feat(checker): add container checkers (docker, containerd, cgroup, runtime)"
```

---

## Task 14: Integration Verification

**Files:** None (verification only)

- [ ] **Step 1: Full build**

Run:
```bash
cd /root/project/opsagent
go build ./...
```
Expected: clean build

- [ ] **Step 2: Run all checker tests**

Run:
```bash
go test ./internal/checker/... -v -count=1 -race
```
Expected: all PASS

- [ ] **Step 3: Run full test suite**

Run:
```bash
go test ./... -count=1 -race
```
Expected: all PASS (existing tests + new checker tests)

- [ ] **Step 4: Verify linter**

Run:
```bash
golangci-lint run ./internal/checker/...
```
Expected: no issues

- [ ] **Step 5: Final commit with any fixes**

If any fixes were needed:
```bash
git add -A
git commit -m "fix(checker): address lint and test issues"
```
