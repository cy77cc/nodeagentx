package service

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
	checker.Register(&UserCheckChecker{})
}

// UserCheckChecker verifies user account status by parsing /etc/passwd and /etc/shadow.
type UserCheckChecker struct{}

func (c *UserCheckChecker) Type() string     { return "user_check" }
func (c *UserCheckChecker) Category() string { return "service" }

type userCheckParams struct {
	Username string `json:"username"`
	Check    string `json:"check"`
}

func (c *UserCheckChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p userCheckParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("user_check: invalid params: %w", err)
	}

	if p.Username == "" {
		return nil, fmt.Errorf("user_check: username is required")
	}

	switch p.Check {
	case "exists":
		return c.checkExists(p.Username)
	case "locked":
		return c.checkLocked(p.Username)
	default:
		return nil, fmt.Errorf("user_check: check must be 'exists' or 'locked', got %q", p.Check)
	}
}

func (c *UserCheckChecker) checkExists(username string) (*checker.CheckResult, error) {
	start := time.Now()

	exists, err := userExistsInPasswd(username)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to parse /etc/passwd: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	result := &checker.CheckResult{
		ExpectedValue: "exists",
		Duration:      time.Since(start),
	}

	if exists {
		result.Status = checker.StatusPass
		result.ActualValue = "exists"
		result.Message = fmt.Sprintf("user %s exists", username)
	} else {
		result.Status = checker.StatusFail
		result.ActualValue = "not_exists"
		result.Message = fmt.Sprintf("user %s does not exist", username)
	}

	return result, nil
}

func (c *UserCheckChecker) checkLocked(username string) (*checker.CheckResult, error) {
	start := time.Now()

	locked, err := userIsLocked(username)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to check lock status: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	result := &checker.CheckResult{
		ExpectedValue: "locked",
		Duration:      time.Since(start),
	}

	if locked {
		result.Status = checker.StatusPass
		result.ActualValue = "locked"
		result.Message = fmt.Sprintf("user %s is locked", username)
	} else {
		result.Status = checker.StatusFail
		result.ActualValue = "unlocked"
		result.Message = fmt.Sprintf("user %s is not locked", username)
	}

	return result, nil
}

// userExistsInPasswd checks if a username exists in /etc/passwd.
func userExistsInPasswd(username string) (bool, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return false, fmt.Errorf("open /etc/passwd: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) >= 1 && parts[0] == username {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read /etc/passwd: %w", err)
	}
	return false, nil
}

// userIsLocked checks if a user account is locked by examining /etc/shadow.
// A locked account has "!" or "!!" prefix, or "*" in the password field.
func userIsLocked(username string) (bool, error) {
	f, err := os.Open("/etc/shadow")
	if err != nil {
		return false, fmt.Errorf("open /etc/shadow: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) >= 2 && parts[0] == username {
			pw := parts[1]
			// Locked accounts have "!" or "!!" prefix, or "*" as the password field.
			if pw == "*" || strings.HasPrefix(pw, "!") {
				return true, nil
			}
			return false, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read /etc/shadow: %w", err)
	}
	return false, fmt.Errorf("user %s not found in /etc/shadow", username)
}
