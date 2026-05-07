package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sys/unix"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&KernelVersionChecker{})
}

// KernelVersionChecker uses syscall.Uname to retrieve the kernel release string.
type KernelVersionChecker struct{}

func (c *KernelVersionChecker) Type() string     { return "kernel_version_check" }
func (c *KernelVersionChecker) Category() string { return "kernel" }

func (c *KernelVersionChecker) Check(_ context.Context, _ json.RawMessage) (*checker.CheckResult, error) {
	start := time.Now()

	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("uname syscall failed: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	// Convert null-terminated byte array to string.
	release := unix.ByteSliceToString(uts.Release[:])

	return &checker.CheckResult{
		Status:      checker.StatusPass,
		ActualValue: release,
		Message:     fmt.Sprintf("kernel version: %s", release),
		Duration:    time.Since(start),
	}, nil
}
