package network

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&NetworkParamChecker{})
}

// NetworkParamChecker converts a sysctl-style key (e.g. "net.ipv4.ip_forward") to
// a /proc/sys/ path and reads the value.
type NetworkParamChecker struct{}

func (c *NetworkParamChecker) Type() string     { return "network_param_check" }
func (c *NetworkParamChecker) Category() string { return "network" }

type networkParamParams struct {
	Key      string `json:"key"`
	Expected string `json:"expected"`
}

func (c *NetworkParamChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p networkParamParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("network_param_check: invalid params: %w", err)
	}

	if p.Key == "" {
		return nil, fmt.Errorf("network_param_check: key is required")
	}

	// Validate key format: must contain only alphanumeric, dots, underscores, hyphens.
	// Reject any path traversal attempts (.., /, \) before conversion.
	if strings.ContainsAny(p.Key, "/\\") {
		return nil, fmt.Errorf("network_param_check: key must not contain path separators, got %q", p.Key)
	}
	for seg := range strings.SplitSeq(p.Key, ".") {
		if seg == ".." || seg == "" {
			return nil, fmt.Errorf("network_param_check: key contains invalid segment, got %q", p.Key)
		}
	}

	// Convert sysctl key to /proc/sys/ path.
	// e.g. "net.ipv4.ip_forward" -> "/proc/sys/net/ipv4/ip_forward"
	procPath := sysctlKeyToProcPath(p.Key)

	// Path traversal prevention: ensure the resolved path is under /proc/sys/.
	cleanPath := filepath.Clean(procPath)
	if !strings.HasPrefix(cleanPath, "/proc/sys/") {
		return nil, fmt.Errorf("network_param_check: key %q resolves to path outside /proc/sys/", p.Key)
	}

	start := time.Now()

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read %s: %v", cleanPath, err),
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
		result.Message = fmt.Sprintf("network param %s = %s (expected)", p.Key, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("network param %s = %s, expected %s", p.Key, actual, p.Expected)
	}

	return result, nil
}

// sysctlKeyToProcPath converts a dot-separated sysctl key to a /proc/sys/ filesystem path.
// For example: "net.ipv4.ip_forward" becomes "/proc/sys/net/ipv4/ip_forward".
func sysctlKeyToProcPath(key string) string {
	// Replace dots with slashes and prepend /proc/sys/.
	return "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
}
