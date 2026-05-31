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
	checker.Register(&BootParamChecker{})
}

// BootParamChecker reads /proc/cmdline and checks for a specific boot parameter.
type BootParamChecker struct{}

func (c *BootParamChecker) Type() string     { return "boot_param_check" }
func (c *BootParamChecker) Category() string { return "kernel" }

type bootParamParams struct {
	Param    string `json:"param"`
	Expected string `json:"expected"`
}

func (c *BootParamChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p bootParamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("boot_param_check: invalid params: %w", err)
	}

	if p.Param == "" {
		return nil, fmt.Errorf("boot_param_check: param is required")
	}

	start := time.Now()

	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read /proc/cmdline: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	cmdline := strings.TrimSpace(string(data))
	actual := parseBootParam(cmdline, p.Param)

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Duration:      time.Since(start),
	}

	if actual == p.Expected {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("boot param %s=%s (expected)", p.Param, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("boot param %s=%s, expected %s", p.Param, actual, p.Expected)
	}

	return result, nil
}

// parseBootParam extracts the value of a boot parameter from the kernel command line.
// Handles "param=value" and bare "param" (returns "1" for bare flags).
func parseBootParam(cmdline, param string) string {
	for part := range strings.FieldsSeq(cmdline) {
		if after, ok := strings.CutPrefix(part, param+"="); ok {
			return after
		}
		if part == param {
			return "1"
		}
	}
	return ""
}
