package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cy77cc/opsagent/internal/checker"
)

func init() {
	checker.Register(&FileExistChecker{})
}

// FileExistChecker verifies that a file exists or does not exist.
type FileExistChecker struct{}

func (c *FileExistChecker) Type() string     { return "file_exist_check" }
func (c *FileExistChecker) Category() string { return "filesystem" }

type fileExistParams struct {
	Path     string `json:"path"`
	Expected string `json:"expected"` // "exists" or "not_exists"
}

func (c *FileExistChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p fileExistParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("file_exist_check: invalid params: %w", err)
	}

	if p.Path == "" {
		return nil, fmt.Errorf("file_exist_check: path is required")
	}

	// Path traversal prevention.
	cleanPath := filepath.Clean(p.Path)
	if cleanPath != p.Path {
		return nil, fmt.Errorf("file_exist_check: path must be clean, got %q", p.Path)
	}

	if p.Expected != "exists" && p.Expected != "not_exists" {
		return nil, fmt.Errorf("file_exist_check: expected must be 'exists' or 'not_exists', got %q", p.Expected)
	}

	start := time.Now()

	_, err := os.Stat(cleanPath)
	actual := "exists"
	if os.IsNotExist(err) {
		actual = "not_exists"
	} else if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to stat %s: %v", cleanPath, err),
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
		result.Message = fmt.Sprintf("file %s is %s (expected)", cleanPath, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("file %s is %s, expected %s", cleanPath, actual, p.Expected)
	}

	return result, nil
}
