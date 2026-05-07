package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&ServiceCheckChecker{})
}

// ServiceCheckChecker verifies the status of a systemd service via systemctl.
type ServiceCheckChecker struct{}

func (c *ServiceCheckChecker) Type() string     { return "service_check" }
func (c *ServiceCheckChecker) Category() string { return "service" }

type serviceCheckParams struct {
	Name           string `json:"name"`
	ExpectedStatus string `json:"expected_status"`
}

func (c *ServiceCheckChecker) Check(ctx context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p serviceCheckParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("service_check: invalid params: %w", err)
	}

	if p.Name == "" {
		return nil, fmt.Errorf("service_check: name is required")
	}

	if p.ExpectedStatus == "" {
		return nil, fmt.Errorf("service_check: expected_status is required")
	}

	start := time.Now()

	out, err := exec.CommandContext(ctx, "systemctl", "is-active", p.Name).Output()
	actual := strings.TrimSpace(string(out))

	if err != nil {
		// systemctl exits non-zero for inactive/failed/unknown states.
		// The output still contains the actual status.
		if actual == "" {
			return &checker.CheckResult{
				Status:   checker.StatusError,
				Message:  fmt.Sprintf("failed to check service %s: %v", p.Name, err),
				Duration: time.Since(start),
			}, nil
		}
	}

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.ExpectedStatus,
		Duration:      time.Since(start),
	}

	if actual == p.ExpectedStatus {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("service %s is %s (expected)", p.Name, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("service %s is %s, expected %s", p.Name, actual, p.ExpectedStatus)
	}

	return result, nil
}
