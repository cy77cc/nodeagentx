package network

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
	checker.Register(&SSHConfigChecker{})
}

// SSHConfigChecker parses /etc/ssh/sshd_config and checks the value of a given key.
type SSHConfigChecker struct{}

func (c *SSHConfigChecker) Type() string     { return "ssh_config_check" }
func (c *SSHConfigChecker) Category() string { return "network" }

type sshConfigParams struct {
	Key      string `json:"key"`
	Expected string `json:"expected"`
}

func (c *SSHConfigChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p sshConfigParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("ssh_config_check: invalid params: %w", err)
	}

	if p.Key == "" {
		return nil, fmt.Errorf("ssh_config_check: key is required")
	}

	start := time.Now()

	actual, err := parseSSHDConfig("/etc/ssh/sshd_config", p.Key)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to parse sshd_config: %v", err),
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
		result.Message = fmt.Sprintf("sshd_config %s = %s (expected)", p.Key, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("sshd_config %s = %s, expected %s", p.Key, actual, p.Expected)
	}

	return result, nil
}

// parseSSHDConfig reads an sshd_config file and returns the last value set for the given key.
// It handles case-insensitive key matching and ignores commented lines.
func parseSSHDConfig(path, key string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	lowerKey := strings.ToLower(key)
	value := ""

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first whitespace.
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}

		if strings.ToLower(parts[0]) == lowerKey {
			value = strings.TrimSpace(parts[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return value, nil
}
