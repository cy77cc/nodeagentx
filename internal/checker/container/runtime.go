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
	checker.Register(&ContainerRuntimeChecker{})
}

// ContainerRuntimeChecker checks whether a container runtime socket exists.
type ContainerRuntimeChecker struct{}

func (c *ContainerRuntimeChecker) Type() string     { return "container_runtime_check" }
func (c *ContainerRuntimeChecker) Category() string { return "container" }

type runtimeParams struct {
	Runtime  string `json:"runtime"`
	Expected string `json:"expected"`
}

// runtimeSockets maps runtime names to their default socket paths.
var runtimeSockets = map[string]string{
	"docker":    "/var/run/docker.sock",
	"containerd": "/var/run/containerd/containerd.sock",
	"cri-o":     "/var/run/crio/crio.sock",
}

func (c *ContainerRuntimeChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p runtimeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("container_runtime_check: invalid params: %w", err)
	}

	if p.Runtime == "" {
		return nil, fmt.Errorf("container_runtime_check: runtime is required")
	}

	if p.Expected == "" {
		p.Expected = "available"
	}

	socketPath, ok := runtimeSockets[p.Runtime]
	if !ok {
		return nil, fmt.Errorf("container_runtime_check: unknown runtime %q (supported: docker, containerd, cri-o)", p.Runtime)
	}

	start := time.Now()

	_, err := os.Stat(socketPath)
	var actual string
	if err == nil {
		actual = "available"
	} else if os.IsNotExist(err) {
		actual = "not_available"
	} else {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to stat %s: %v", socketPath, err),
			Duration: time.Since(start),
		}, nil
	}

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Duration:      time.Since(start),
	}

	if actual == p.Expected {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("container runtime %s is %s (expected)", p.Runtime, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("container runtime %s is %s, expected %s", p.Runtime, actual, p.Expected)
	}

	return result, nil
}
