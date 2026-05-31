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
	checker.Register(&DockerChecker{})
}

// DockerChecker reads /etc/docker/daemon.json and searches for a key/value pair.
type DockerChecker struct{}

func (c *DockerChecker) Type() string     { return "docker_check" }
func (c *DockerChecker) Category() string { return "container" }

type dockerParams struct {
	Check    string `json:"check"`
	Key      string `json:"key"`
	Expected string `json:"expected"`
}

func (c *DockerChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p dockerParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("docker_check: invalid params: %w", err)
	}

	if p.Key == "" {
		return nil, fmt.Errorf("docker_check: key is required")
	}

	start := time.Now()

	data, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read /etc/docker/daemon.json: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	content := string(data)

	// Simple string search: look for the key in the JSON content.
	// Format expected: "key": "value"
	searchKey := fmt.Sprintf("%q", p.Key)
	if !strings.Contains(content, searchKey) {
		return &checker.CheckResult{
			ActualValue:   "key_not_found",
			ExpectedValue: p.Expected,
			Status:        checker.StatusFail,
			Message:       fmt.Sprintf("docker key %q not found in daemon.json", p.Key),
			Duration:      time.Since(start),
		}, nil
	}

	// Attempt to extract the value by parsing JSON into a map.
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to parse /etc/docker/daemon.json: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	val, ok := config[p.Key]
	if !ok {
		return &checker.CheckResult{
			ActualValue:   "key_not_found",
			ExpectedValue: p.Expected,
			Status:        checker.StatusFail,
			Message:       fmt.Sprintf("docker key %q not found in daemon.json", p.Key),
			Duration:      time.Since(start),
		}, nil
	}

	actual := fmt.Sprintf("%v", val)
	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.Expected,
		Duration:      time.Since(start),
	}

	if actual == p.Expected {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("docker %s = %s (expected)", p.Key, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("docker %s = %s, expected %s", p.Key, actual, p.Expected)
	}

	return result, nil
}
