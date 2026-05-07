package network

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
	checker.Register(&IPTablesChecker{})
}

// IPTablesChecker runs `iptables -L <chain> -n` and parses the policy from the first line.
type IPTablesChecker struct{}

func (c *IPTablesChecker) Type() string     { return "iptables_check" }
func (c *IPTablesChecker) Category() string { return "network" }

type iptablesParams struct {
	Chain          string `json:"chain"`
	ExpectedPolicy string `json:"expected_policy"`
}

func (c *IPTablesChecker) Check(ctx context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p iptablesParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("iptables_check: invalid params: %w", err)
	}

	if p.Chain == "" {
		return nil, fmt.Errorf("iptables_check: chain is required")
	}

	// Validate chain name to prevent injection.
	upper := strings.ToUpper(p.Chain)
	if upper != "INPUT" && upper != "OUTPUT" && upper != "FORWARD" {
		return nil, fmt.Errorf("iptables_check: chain must be INPUT, OUTPUT, or FORWARD, got %q", p.Chain)
	}

	start := time.Now()

	policy, err := getIPTablesPolicy(ctx, upper)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to get iptables policy for chain %s: %v", upper, err),
			Duration: time.Since(start),
		}, nil
	}

	result := &checker.CheckResult{
		ActualValue:   policy,
		ExpectedValue: p.ExpectedPolicy,
		Duration:      time.Since(start),
	}

	if policy == p.ExpectedPolicy {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("iptables chain %s policy = %s (expected)", upper, policy)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("iptables chain %s policy = %s, expected %s", upper, policy, p.ExpectedPolicy)
	}

	return result, nil
}

// getIPTablesPolicy runs iptables -L <chain> -n and extracts the policy from the first line.
// The first line format: "Chain INPUT (policy DROP)" or "Chain INPUT (0 references)" for custom chains.
func getIPTablesPolicy(ctx context.Context, chain string) (string, error) {
	cmd := exec.CommandContext(ctx, "iptables", "-L", chain, "-n")
	out, err := cmd.Output()
	if err != nil {
		// Check if it's an exit error (iptables not available or permission denied).
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("iptables exited with code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("iptables exec: %w", err)
	}

	// Parse the first line.
	firstLine := ""
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			firstLine = trimmed
			break
		}
	}

	if firstLine == "" {
		return "", fmt.Errorf("iptables returned empty output")
	}

	// Extract policy from "Chain INPUT (policy DROP)" format.
	if idx := strings.Index(firstLine, "(policy "); idx != -1 {
		rest := firstLine[idx+len("(policy "):]
		if endIdx := strings.Index(rest, ")"); endIdx != -1 {
			return rest[:endIdx], nil
		}
	}

	// If no policy found, it's a custom chain with "(N references)".
	return "no policy (custom chain)", nil
}
