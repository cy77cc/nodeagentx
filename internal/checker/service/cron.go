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
	checker.Register(&CronCheckChecker{})
}

// CronCheckChecker reads a user's crontab and counts non-comment, non-empty entries.
type CronCheckChecker struct{}

func (c *CronCheckChecker) Type() string     { return "cron_check" }
func (c *CronCheckChecker) Category() string { return "service" }

type cronCheckParams struct {
	User string `json:"user"`
}

func (c *CronCheckChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p cronCheckParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("cron_check: invalid params: %w", err)
	}

	if p.User == "" {
		return nil, fmt.Errorf("cron_check: user is required")
	}

	start := time.Now()

	count, err := countCronEntries(p.User)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read crontab for %s: %v", p.User, err),
			Duration: time.Since(start),
		}, nil
	}

	return &checker.CheckResult{
		Status:      checker.StatusPass,
		ActualValue: fmt.Sprintf("%d", count),
		Message:     fmt.Sprintf("user %s has %d cron entries", p.User, count),
		Duration:    time.Since(start),
	}, nil
}

// countCronEntries reads /var/spool/cron/crontabs/<user> and counts
// non-comment, non-empty lines.
func countCronEntries(user string) (int, error) {
	path := fmt.Sprintf("/var/spool/cron/crontabs/%s", user)

	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	return count, nil
}
