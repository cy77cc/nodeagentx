package filesystem

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
	checker.Register(&MountOptionChecker{})
}

// MountOptionChecker parses /proc/mounts to verify a mount point has an expected option.
type MountOptionChecker struct{}

func (c *MountOptionChecker) Type() string     { return "mount_option_check" }
func (c *MountOptionChecker) Category() string { return "filesystem" }

type mountOptionParams struct {
	MountPoint     string `json:"mount_point"`
	ExpectedOption string `json:"expected_option"`
}

func (c *MountOptionChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p mountOptionParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("mount_option_check: invalid params: %w", err)
	}

	if p.MountPoint == "" {
		return nil, fmt.Errorf("mount_option_check: mount_point is required")
	}
	if p.ExpectedOption == "" {
		return nil, fmt.Errorf("mount_option_check: expected_option is required")
	}

	start := time.Now()

	options, err := findMountOptions(p.MountPoint)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read /proc/mounts: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	if options == "" {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("mount point %s not found in /proc/mounts", p.MountPoint),
			Duration: time.Since(start),
		}, nil
	}

	actual := "absent"
	if hasMountOption(options, p.ExpectedOption) {
		actual = "present"
	}

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: "present",
		Duration:      time.Since(start),
	}

	if actual == "present" {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("mount %s has option %s (expected)", p.MountPoint, p.ExpectedOption)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("mount %s missing option %s (have: %s)", p.MountPoint, p.ExpectedOption, options)
	}

	return result, nil
}

// findMountOptions reads /proc/mounts and returns the mount options for the given mount point.
func findMountOptions(mountPoint string) (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Format: device mountpoint fstype options dump pass
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 && fields[1] == mountPoint {
			return fields[3], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

// hasMountOption checks if a specific option is present in a comma-separated options string.
func hasMountOption(options, option string) bool {
	for _, opt := range strings.Split(options, ",") {
		if opt == option {
			return true
		}
	}
	return false
}
