package kernel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&KernelModuleChecker{})
}

// KernelModuleChecker reads /proc/modules to check if a kernel module is loaded.
type KernelModuleChecker struct{}

func (c *KernelModuleChecker) Type() string     { return "kernel_module_check" }
func (c *KernelModuleChecker) Category() string { return "kernel" }

type moduleParams struct {
	Module   string `json:"module"`
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

	start := time.Now()

	loaded, err := isModuleLoaded(p.Module)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read /proc/modules: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	actualStatus := "not_loaded"
	if loaded {
		actualStatus = "loaded"
	}

	result := &checker.CheckResult{
		ActualValue:   actualStatus,
		ExpectedValue: p.Expected,
		Duration:      time.Since(start),
	}

	if actualStatus == p.Expected {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("module %s is %s (expected)", p.Module, actualStatus)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("module %s is %s, expected %s", p.Module, actualStatus, p.Expected)
	}

	return result, nil
}

// isModuleLoaded reads /proc/modules and checks if the given module name appears.
func isModuleLoaded(name string) (bool, error) {
	f, err := os.Open("/proc/modules")
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Each line starts with the module name followed by a space.
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 && fields[0] == name {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}
