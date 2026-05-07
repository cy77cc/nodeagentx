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
	checker.Register(&FilePermChecker{})
}

// FilePermChecker verifies that a file has the expected permission mode.
type FilePermChecker struct{}

func (c *FilePermChecker) Type() string     { return "file_perm_check" }
func (c *FilePermChecker) Category() string { return "filesystem" }

type filePermParams struct {
	Path         string `json:"path"`
	ExpectedMode string `json:"expected_mode"`
}

func (c *FilePermChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p filePermParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("file_perm_check: invalid params: %w", err)
	}

	if p.Path == "" {
		return nil, fmt.Errorf("file_perm_check: path is required")
	}

	// Path traversal prevention.
	cleanPath := filepath.Clean(p.Path)
	if cleanPath != p.Path {
		return nil, fmt.Errorf("file_perm_check: path must be clean, got %q", p.Path)
	}

	start := time.Now()

	info, err := os.Lstat(cleanPath)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to stat %s: %v", cleanPath, err),
			Duration: time.Since(start),
		}, nil
	}

	actual := fmt.Sprintf("%04o", info.Mode().Perm())

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.ExpectedMode,
		Duration:      time.Since(start),
	}

	if actual == p.ExpectedMode {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("file %s mode %s (expected)", cleanPath, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("file %s mode %s, expected %s", cleanPath, actual, p.ExpectedMode)
	}

	return result, nil
}
