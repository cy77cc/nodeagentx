package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&CgroupChecker{})
}

// CgroupChecker detects the cgroup version by checking filesystem markers.
type CgroupChecker struct{}

func (c *CgroupChecker) Type() string     { return "cgroup_check" }
func (c *CgroupChecker) Category() string { return "container" }

func (c *CgroupChecker) Check(_ context.Context, _ json.RawMessage) (*checker.CheckResult, error) {
	start := time.Now()

	// cgroup v2: presence of /sys/fs/cgroup/cgroup.controllers indicates unified hierarchy.
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return &checker.CheckResult{
			ActualValue:   "v2",
			ExpectedValue: "v2",
			Status:        checker.StatusPass,
			Message:       "cgroup v2 (unified hierarchy) detected",
			Duration:      time.Since(start),
		}, nil
	}

	// cgroup v1: presence of /sys/fs/cgroup/cpu indicates legacy hierarchy.
	if _, err := os.Stat("/sys/fs/cgroup/cpu"); err == nil {
		return &checker.CheckResult{
			ActualValue:   "v1",
			ExpectedValue: "v1",
			Status:        checker.StatusPass,
			Message:       "cgroup v1 (legacy hierarchy) detected",
			Duration:      time.Since(start),
		}, nil
	}

	return &checker.CheckResult{
		Status:   checker.StatusError,
		Message:  fmt.Sprintf("unable to detect cgroup version: %v", fmt.Errorf("no cgroup markers found")),
		Duration: time.Since(start),
	}, nil
}
