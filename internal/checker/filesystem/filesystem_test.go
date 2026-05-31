package filesystem

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

// --- FilePermChecker tests ---

func TestFilePermCheckerTypeAndCategory(t *testing.T) {
	c := &FilePermChecker{}
	assert.Equal(t, "file_perm_check", c.Type())
	assert.Equal(t, "filesystem", c.Category())
}

func TestFilePermCheckerPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

	params, _ := json.Marshal(map[string]string{
		"path":         path,
		"expected_mode": "0644",
	})
	c := &FilePermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "0644", result.ActualValue)
}

func TestFilePermCheckerFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o755))

	params, _ := json.Marshal(map[string]string{
		"path":         path,
		"expected_mode": "0644",
	})
	c := &FilePermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
	assert.Equal(t, "0755", result.ActualValue)
}

func TestFilePermCheckerFileNotFound(t *testing.T) {
	params := json.RawMessage(`{"path": "/nonexistent/path/file", "expected_mode": "0644"}`)
	c := &FilePermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
}

func TestFilePermCheckerEmptyPath(t *testing.T) {
	params := json.RawMessage(`{"path": "", "expected_mode": "0644"}`)
	c := &FilePermChecker{}
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestFilePermCheckerInvalidJSON(t *testing.T) {
	c := &FilePermChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
}

func TestFilePermCheckerPathTraversal(t *testing.T) {
	c := &FilePermChecker{}
	params := json.RawMessage(`{"path": "/tmp/../etc/shadow", "expected_mode": "0640"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path must be clean")
}

func TestFilePermCheckerOctalFormatting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))
	require.NoError(t, os.Chmod(path, 0o7))

	params, _ := json.Marshal(map[string]string{
		"path":         path,
		"expected_mode": "0007",
	})
	c := &FilePermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "0007", result.ActualValue)
}

// --- FileExistChecker tests ---

func TestFileExistCheckerTypeAndCategory(t *testing.T) {
	c := &FileExistChecker{}
	assert.Equal(t, "file_exist_check", c.Type())
	assert.Equal(t, "filesystem", c.Category())
}

func TestFileExistCheckerExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

	params, _ := json.Marshal(map[string]string{
		"path":     path,
		"expected": "exists",
	})
	c := &FileExistChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "exists", result.ActualValue)
}

func TestFileExistCheckerNotExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent")

	params, _ := json.Marshal(map[string]string{
		"path":     path,
		"expected": "not_exists",
	})
	c := &FileExistChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "not_exists", result.ActualValue)
}

func TestFileExistCheckerExistsButExpectedNot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

	params, _ := json.Marshal(map[string]string{
		"path":     path,
		"expected": "not_exists",
	})
	c := &FileExistChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
}

func TestFileExistCheckerEmptyPath(t *testing.T) {
	params := json.RawMessage(`{"path": "", "expected": "exists"}`)
	c := &FileExistChecker{}
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestFileExistCheckerInvalidExpected(t *testing.T) {
	params := json.RawMessage(`{"path": "/tmp/testfile", "expected": "maybe"}`)
	c := &FileExistChecker{}
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exists")
}

func TestFileExistCheckerInvalidJSON(t *testing.T) {
	c := &FileExistChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestFileExistCheckerPathTraversal(t *testing.T) {
	c := &FileExistChecker{}
	params := json.RawMessage(`{"path": "/tmp/../etc/shadow", "expected": "exists"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path must be clean")
}

// --- DirPermChecker tests ---

func TestDirPermCheckerTypeAndCategory(t *testing.T) {
	c := &DirPermChecker{}
	assert.Equal(t, "dir_perm_check", c.Type())
	assert.Equal(t, "filesystem", c.Category())
}

func TestDirPermCheckerPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(path, 0o755))

	params, _ := json.Marshal(map[string]string{
		"path":         path,
		"expected_mode": "0755",
	})
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "0755", result.ActualValue)
}

func TestDirPermCheckerFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(path, 0o700))

	params, _ := json.Marshal(map[string]string{
		"path":         path,
		"expected_mode": "0755",
	})
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
	assert.Equal(t, "0700", result.ActualValue)
}

func TestDirPermCheckerNotADirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "afile")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o644))

	params, _ := json.Marshal(map[string]string{
		"path":         path,
		"expected_mode": "0755",
	})
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
	assert.Contains(t, result.Message, "not a directory")
}

func TestDirPermCheckerNotFound(t *testing.T) {
	params := json.RawMessage(`{"path": "/nonexistent/dir", "expected_mode": "0755"}`)
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
}

func TestDirPermCheckerEmptyPath(t *testing.T) {
	params := json.RawMessage(`{"path": "", "expected_mode": "0755"}`)
	c := &DirPermChecker{}
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path is required")
}

func TestDirPermCheckerInvalidJSON(t *testing.T) {
	c := &DirPermChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`bad`))
	require.Error(t, err)
}

func TestDirPermCheckerPathTraversal(t *testing.T) {
	c := &DirPermChecker{}
	params := json.RawMessage(`{"path": "/tmp/../etc", "expected_mode": "0755"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "path must be clean")
}

func TestDirPermCheckerStickyBitExpected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stickydir")
	require.NoError(t, os.Mkdir(path, 0o777))
	require.NoError(t, os.Chmod(path, os.ModeSticky|0o777))

	stickyTrue := true
	params, _ := json.Marshal(map[string]any{
		"path":         path,
		"expected_mode": "1777",
		"sticky_bit":   &stickyTrue,
	})
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Mode is 1777, sticky bit present: pass.
	assert.Equal(t, checker.StatusPass, result.Status)
}

func TestDirPermCheckerStickyBitMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nosticky")
	require.NoError(t, os.Mkdir(path, 0o777))

	stickyTrue := true
	params, _ := json.Marshal(map[string]any{
		"path":         path,
		"expected_mode": "1777",
		"sticky_bit":   &stickyTrue,
	})
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
}

func TestDirPermCheckerStickyBitNotExpected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nosticky")
	require.NoError(t, os.Mkdir(path, 0o755))

	stickyFalse := false
	params, _ := json.Marshal(map[string]any{
		"path":         path,
		"expected_mode": "0755",
		"sticky_bit":   &stickyFalse,
	})
	c := &DirPermChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
}

// --- MountOptionChecker tests ---

func TestMountOptionCheckerTypeAndCategory(t *testing.T) {
	c := &MountOptionChecker{}
	assert.Equal(t, "mount_option_check", c.Type())
	assert.Equal(t, "filesystem", c.Category())
}

func TestMountOptionCheckerInvalidJSON(t *testing.T) {
	c := &MountOptionChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`{bad`))
	require.Error(t, err)
}

func TestMountOptionCheckerEmptyMountPoint(t *testing.T) {
	params := json.RawMessage(`{"mount_point": "", "expected_option": "rw"}`)
	c := &MountOptionChecker{}
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mount_point is required")
}

func TestMountOptionCheckerEmptyOption(t *testing.T) {
	params := json.RawMessage(`{"mount_point": "/", "expected_option": ""}`)
	c := &MountOptionChecker{}
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected_option is required")
}

func TestMountOptionCheckerRootMount(t *testing.T) {
	if _, err := os.Stat("/proc/mounts"); err != nil {
		t.Skip("no /proc/mounts available")
	}

	params := json.RawMessage(`{"mount_point": "/", "expected_option": "rw"}`)
	c := &MountOptionChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Root is typically rw.
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "present", result.ActualValue)
}

func TestMountOptionCheckerMissingOption(t *testing.T) {
	if _, err := os.Stat("/proc/mounts"); err != nil {
		t.Skip("no /proc/mounts available")
	}

	params := json.RawMessage(`{"mount_point": "/", "expected_option": "nonexistent_option_xyz"}`)
	c := &MountOptionChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusFail, result.Status)
	assert.Equal(t, "absent", result.ActualValue)
}

func TestMountOptionCheckerNonexistentMountPoint(t *testing.T) {
	if _, err := os.Stat("/proc/mounts"); err != nil {
		t.Skip("no /proc/mounts available")
	}

	params := json.RawMessage(`{"mount_point": "/nonexistent_mount_point_xyz", "expected_option": "rw"}`)
	c := &MountOptionChecker{}
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
	assert.Contains(t, result.Message, "not found")
}

// --- HasMountOption unit tests ---

func TestHasMountOption(t *testing.T) {
	assert.True(t, hasMountOption("rw,relatime,seclabel", "rw"))
	assert.True(t, hasMountOption("rw,relatime,seclabel", "relatime"))
	assert.False(t, hasMountOption("rw,relatime,seclabel", "ro"))
	assert.False(t, hasMountOption("", "rw"))
}

// --- Registration test ---

func TestFilesystemCheckersRegistered(t *testing.T) {
	types := []string{"file_perm_check", "file_exist_check", "dir_perm_check", "mount_option_check"}
	for _, typ := range types {
		_, ok := checker.DefaultRegistry.Get(typ)
		assert.True(t, ok, "checker %q should be registered", typ)
	}
}
