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
	checker.Register(&DirPermChecker{})
}

// DirPermChecker verifies that a directory has the expected permission mode,
// with an optional sticky bit check.
type DirPermChecker struct{}

func (c *DirPermChecker) Type() string     { return "dir_perm_check" }
func (c *DirPermChecker) Category() string { return "filesystem" }

type dirPermParams struct {
	Path         string `json:"path"`
	ExpectedMode string `json:"expected_mode"`
	StickyBit    *bool  `json:"sticky_bit,omitzero"` // optional
}

func (c *DirPermChecker) Check(_ context.Context, params json.RawMessage) (*checker.CheckResult, error) {
	var p dirPermParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("dir_perm_check: invalid params: %w", err)
	}

	if p.Path == "" {
		return nil, fmt.Errorf("dir_perm_check: path is required")
	}

	// Path traversal prevention.
	cleanPath := filepath.Clean(p.Path)
	if cleanPath != p.Path {
		return nil, fmt.Errorf("dir_perm_check: path must be clean, got %q", p.Path)
	}

	start := time.Now()

	info, err := os.Stat(cleanPath)
	if err != nil {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("failed to stat %s: %v", cleanPath, err),
			Duration: time.Since(start),
		}, nil
	}

	if !info.IsDir() {
		return &checker.CheckResult{
			Status:   checker.StatusError,
			Message:  fmt.Sprintf("%s is not a directory", cleanPath),
			Duration: time.Since(start),
		}, nil
	}

	// Format mode as a 4-digit octal including special bits (setuid/setgid/sticky).
	actual := formatDirMode(info.Mode())

	result := &checker.CheckResult{
		ActualValue:   actual,
		ExpectedValue: p.ExpectedMode,
		Duration:      time.Since(start),
	}

	pass := actual == p.ExpectedMode

	// Check sticky bit if requested.
	// In Go, os.ModeSticky is in the high bits of FileMode, not in Perm().
	if p.StickyBit != nil {
		hasSticky := info.Mode()&os.ModeSticky != 0
		if *p.StickyBit != hasSticky {
			pass = false
			stickyStatus := "present"
			if !hasSticky {
				stickyStatus = "absent"
			}
			result.ActualValue = fmt.Sprintf("%s (sticky=%s)", actual, stickyStatus)
			result.ExpectedValue = fmt.Sprintf("%s (sticky=%v)", p.ExpectedMode, *p.StickyBit)
		}
	}

	if pass {
		result.Status = checker.StatusPass
		result.Message = fmt.Sprintf("directory %s mode %s (expected)", cleanPath, actual)
	} else {
		result.Status = checker.StatusFail
		result.Message = fmt.Sprintf("directory %s mode %s, expected %s", cleanPath, actual, p.ExpectedMode)
	}

	return result, nil
}

// formatDirMode formats an os.FileMode as a 4-digit octal string including
// the setuid, setgid, and sticky bits.
func formatDirMode(m os.FileMode) string {
	mode := uint32(m.Perm())
	if m&os.ModeSetuid != 0 {
		mode |= 0o4000
	}
	if m&os.ModeSetgid != 0 {
		mode |= 0o2000
	}
	if m&os.ModeSticky != 0 {
		mode |= 0o1000
	}
	return fmt.Sprintf("%04o", mode)
}
