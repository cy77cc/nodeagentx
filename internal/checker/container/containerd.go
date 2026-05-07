package container

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
	checker.Register(&ContainerdChecker{})
}

// ContainerdChecker reads /etc/containerd/config.toml and checks for a key/value.
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

	start := time.Now()

	data, err := os.ReadFile("/etc/containerd/config.toml")
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read /etc/containerd/config.toml: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	var actual string
	found := false

	// Simple TOML key search: look for lines matching "key = value".
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Match "key = value" or "key=value" patterns.
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}

		lineKey := strings.TrimSpace(parts[0])
		if lineKey == p.Key {
			val := strings.TrimSpace(parts[1])
			// Strip surrounding quotes if present.
			val = strings.Trim(val, "\"")
			actual = val
			found = true
			break
		}
	}

	if !found {
		return &checker.CheckResult{
			ActualValue:   "key_not_found",
			ExpectedValue: p.Expected,
			Status:        checker.StatusFail,
			Message:       fmt.Sprintf("containerd key %q not found in config.toml", p.Key),
			Duration:      time.Since(start),
		}, nil
	}

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Duration:      time.Since(start),
	}

	if actual == p.Expected {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("containerd %s = %s (expected)", p.Key, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("containerd %s = %s, expected %s", p.Key, actual, p.Expected)
	}

	return result, nil
}
