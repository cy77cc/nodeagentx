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
	checker.Register(&PAMCheckChecker{})
}

// PAMCheckChecker verifies that a PAM module is referenced in a given PAM config file.
type PAMCheckChecker struct{}

func (c *PAMCheckChecker) Type() string     { return "pam_check" }
func (c *PAMCheckChecker) Category() string { return "service" }

type pamCheckParams struct {
	Module string `json:"module"`
	File   string `json:"file"`
}

func (c *PAMCheckChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p pamCheckParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("pam_check: invalid params: %w", err)
	}

	if p.Module == "" {
		return nil, fmt.Errorf("pam_check: module is required")
	}

	if p.File == "" {
		return nil, fmt.Errorf("pam_check: file is required")
	}

	start := time.Now()

	found, err := pamModuleInFile(p.Module, p.File)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to read PAM config: %v", err),
			Duration: time.Since(start),
		}, nil
	}

	result := &checker.CheckResult{
		ExpectedValue: "present",
		Duration:      time.Since(start),
	}

	if found {
		result.Status = checker.StatusPass
		result.ActualValue = "present"
		result.Message = fmt.Sprintf("module %s found in /etc/pam.d/%s", p.Module, p.File)
	} else {
		result.Status = checker.StatusFail
		result.ActualValue = "absent"
		result.Message = fmt.Sprintf("module %s not found in /etc/pam.d/%s", p.Module, p.File)
	}

	return result, nil
}

// pamModuleInFile checks whether the given module string appears in
// /etc/pam.d/<file>. It skips comments and empty lines.
func pamModuleInFile(module, file string) (bool, error) {
	path := fmt.Sprintf("/etc/pam.d/%s", file)

	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, module) {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return false, nil
}
