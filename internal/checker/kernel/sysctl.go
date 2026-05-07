package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&SysctlChecker{})
}

// SysctlChecker reads a value from /proc/sys/ and compares it to an expected value.
type SysctlChecker struct{}

func (c *SysctlChecker) Type() string     { return "sysctl_check" }
func (c *SysctlChecker) Category() string { return "kernel" }

type sysctlParams struct {
	Path     string `json:"path"`
	Expected string `json:"expected"`
}

func (c *SysctlChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p sysctlParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("sysctl_check: invalid params: %w", err)
	}

	if p.Path == "" {
		return nil, fmt.Errorf("sysctl_check: path is required")
	}

	// Path traversal prevention: must start with /proc/sys/
	if !strings.HasPrefix(p.Path, "/proc/sys/") {
		return nil, fmt.Errorf("sysctl_check: path must start with /proc/sys/, got %q", p.Path)
	}

	start := time.Now()

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read %s: %v", p.Path, err),
			Duration: time.Since(start),
		}, nil
	}

	actual := strings.TrimSpace(string(data))

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Duration:      time.Since(start),
	}

	if actual == p.Expected {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("sysctl %s = %s (expected)", p.Path, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("sysctl %s = %s, expected %s", p.Path, actual, p.Expected)
	}

	return result, nil
}
