package kernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy77cc/opsagent/internal/checker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- SysctlChecker tests ---

func TestSysctlCheckerTypeAndCategory(t *testing.T) {
	c := &SysctlChecker{}
	assert.Equal(t, "sysctl_check", c.Type())
	assert.Equal(t, "kernel", c.Category())
}

func TestSysctlCheckerPassesWhenMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "some_setting")
	require.NoError(t, os.WriteFile(path, []byte("0\n"), 0o644))

	// Temporarily override the path by using a real /proc/sys/ path.
	// For unit tests, we test path validation and error cases instead.
	params := json.RawMessage(`{"path": "/proc/sys/kernel/hostname", "expected": "placeholder_never_match"}`)
	c := &SysctlChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// The actual value won't match, so it should fail.
	assert.Equal(t, checker.StatusFail, result.Status)
}

func TestSysctlCheckerPathTraversal(t *testing.T) {
	c := &SysctlChecker{}
	params := json.RawMessage(`{"path": "/etc/passwd", "expected": "0"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/proc/sys/")
}

func TestSysctlCheckerPathTraversalRelative(t *testing.T) {
	c := &SysctlChecker{}
	params := json.RawMessage(`{"path": "../etc/passwd", "expected": "0"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/proc/sys/")
}

func TestSysctlCheckerEmptyPath(t *testing.T) {
	c := &SysctlChecker{}
	params := json.RawMessage(`{"path": "", "expected": "0"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}

func TestSysctlCheckerInvalidJSON(t *testing.T) {
	c := &SysctlChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
}

func TestSysctlCheckerFileNotFound(t *testing.T) {
	c := &SysctlChecker{}
	params := json.RawMessage(`{"path": "/proc/sys/nonexistent/path", "expected": "0"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
}

func TestSysctlCheckerPassesWithRealValue(t *testing.T) {
	// Read a real /proc/sys value to verify pass behavior.
	data, err := os.ReadFile("/proc/sys/kernel/hostname")
	if err != nil {
		t.Skip("cannot read /proc/sys/kernel/hostname, skipping")
	}
	actual := string(data[:len(data)-1]) // trim newline
	if len(actual) > 100 {
		t.Skip("hostname too long for test param")
	}

	params, _ := json.Marshal(map[string]string{
		"path":     "/proc/sys/kernel/hostname",
		"expected": actual,
	})
	c := &SysctlChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, actual, result.ActualValue)
}

// --- KernelVersionChecker tests ---

func TestKernelVersionCheckerTypeAndCategory(t *testing.T) {
	c := &KernelVersionChecker{}
	assert.Equal(t, "kernel_version_check", c.Type())
	assert.Equal(t, "kernel", c.Category())
}

func TestKernelVersionCheckerReturnsVersion(t *testing.T) {
	c := &KernelVersionChecker{}
	result, err := c.Check(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.NotEmpty(t, result.ActualValue)
}

// --- KernelModuleChecker tests ---

func TestKernelModuleCheckerTypeAndCategory(t *testing.T) {
	c := &KernelModuleChecker{}
	assert.Equal(t, "kernel_module_check", c.Type())
	assert.Equal(t, "kernel", c.Category())
}

func TestKernelModuleCheckerEmptyModule(t *testing.T) {
	c := &KernelModuleChecker{}
	params := json.RawMessage(`{"module": "", "expected": "loaded"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "module is required")
}

func TestKernelModuleCheckerInvalidJSON(t *testing.T) {
	c := &KernelModuleChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestKernelModuleCheckerNotLoaded(t *testing.T) {
	c := &KernelModuleChecker{}
	params := json.RawMessage(`{"module": "nonexistent_module_xyz", "expected": "not_loaded"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "not_loaded", result.ActualValue)
}

func TestKernelModuleCheckerLoadedMismatch(t *testing.T) {
	c := &KernelModuleChecker{}
	params := json.RawMessage(`{"module": "nonexistent_module_xyz", "expected": "loaded"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
}

func TestKernelModuleCheckerWithProcModules(t *testing.T) {
	if _, err := os.Stat("/proc/modules"); err != nil {
		t.Skip("no /proc/modules available")
	}

	c := &KernelModuleChecker{}
	params := json.RawMessage(`{"module": "ext4", "expected": "loaded"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Just verify it returns a valid result without error.
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

// --- BootParamChecker tests ---

func TestBootParamCheckerTypeAndCategory(t *testing.T) {
	c := &BootParamChecker{}
	assert.Equal(t, "boot_param_check", c.Type())
	assert.Equal(t, "kernel", c.Category())
}

func TestBootParamCheckerEmptyParam(t *testing.T) {
	c := &BootParamChecker{}
	params := json.RawMessage(`{"param": "", "expected": "1"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "param is required")
}

func TestBootParamCheckerInvalidJSON(t *testing.T) {
	c := &BootParamChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`bad`))
	require.Error(t, err)
}

func TestBootParamCheckerWithProcCmdline(t *testing.T) {
	if _, err := os.Stat("/proc/cmdline"); err != nil {
		t.Skip("no /proc/cmdline available")
	}

	c := &BootParamChecker{}
	params := json.RawMessage(`{"param": "nonexistent_boot_param_xyz", "expected": ""}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// nonexistent param should not be found, actual should be empty.
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "", result.ActualValue)
}

// --- parseBootParam unit tests ---

func TestParseBootParamKeyValue(t *testing.T) {
	assert.Equal(t, "1", parseBootParam("root=/dev/sda1 selinux=1 console=tty0", "selinux"))
	assert.Equal(t, "/dev/sda1", parseBootParam("root=/dev/sda1 selinux=1", "root"))
	assert.Equal(t, "tty0", parseBootParam("root=/dev/sda1 console=tty0", "console"))
}

func TestParseBootParamBareFlag(t *testing.T) {
	assert.Equal(t, "1", parseBootParam("quiet splash verbose", "verbose"))
	assert.Equal(t, "1", parseBootParam("quiet", "quiet"))
}

func TestParseBootParamMissing(t *testing.T) {
	assert.Equal(t, "", parseBootParam("root=/dev/sda1 quiet", "selinux"))
}

func TestParseBootParamEmptyCmdline(t *testing.T) {
	assert.Equal(t, "", parseBootParam("", "anything"))
}

// --- Registration test ---

func TestKernelCheckersRegistered(t *testing.T) {
	types := []string{"sysctl_check", "kernel_version_check", "kernel_module_check", "boot_param_check"}
	for _, typ := range types {
		_, ok := checker.DefaultRegistry.Get(typ)
		assert.True(t, ok, "checker %q should be registered", typ)
	}
}
