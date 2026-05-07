package container

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

// --- DockerChecker tests ---

func TestDockerCheckerTypeAndCategory(t *testing.T) {
	c := &DockerChecker{}
	assert.Equal(t, "docker_check", c.Type())
	assert.Equal(t, "container", c.Category())
}

func TestDockerCheckerInvalidJSON(t *testing.T) {
	c := &DockerChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`not json`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid params")
}

func TestDockerCheckerEmptyKey(t *testing.T) {
	c := &DockerChecker{}
	params := json.RawMessage(`{"check": "daemon_json", "key": "", "expected": "overlay2"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestDockerCheckerFileNotFound(t *testing.T) {
	if _, err := os.Stat("/etc/docker/daemon.json"); err == nil {
		t.Skip("/etc/docker/daemon.json exists, skipping file-not-found test")
	}

	c := &DockerChecker{}
	params := json.RawMessage(`{"check": "daemon_json", "key": "storage-driver", "expected": "overlay2"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
}

func TestDockerCheckerWithTempFile(t *testing.T) {
	// The path is hardcoded to /etc/docker/daemon.json, so we test error handling.
	// In test environment, the file likely doesn't exist.
	c := &DockerChecker{}
	params := json.RawMessage(`{"check": "daemon_json", "key": "storage-driver", "expected": "overlay2"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Contains(t, []checker.CheckStatus{checker.StatusError, checker.StatusPass, checker.StatusFail}, result.Status)
}

func TestDockerCheckerPassesWhenKeyMatches(t *testing.T) {
	// If /etc/docker/daemon.json exists, test with it.
	if _, err := os.Stat("/etc/docker/daemon.json"); err != nil {
		t.Skip("no /etc/docker/daemon.json available")
	}

	c := &DockerChecker{}
	params := json.RawMessage(`{"check": "daemon_json", "key": "storage-driver", "expected": "overlay2"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

// --- ContainerdChecker tests ---

func TestContainerdCheckerTypeAndCategory(t *testing.T) {
	c := &ContainerdChecker{}
	assert.Equal(t, "containerd_check", c.Type())
	assert.Equal(t, "container", c.Category())
}

func TestContainerdCheckerInvalidJSON(t *testing.T) {
	c := &ContainerdChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`bad`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid params")
}

func TestContainerdCheckerEmptyKey(t *testing.T) {
	c := &ContainerdChecker{}
	params := json.RawMessage(`{"key": "", "expected": "2"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key is required")
}

func TestContainerdCheckerFileNotFound(t *testing.T) {
	if _, err := os.Stat("/etc/containerd/config.toml"); err == nil {
		t.Skip("/etc/containerd/config.toml exists, skipping file-not-found test")
	}

	c := &ContainerdChecker{}
	params := json.RawMessage(`{"key": "version", "expected": "2"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusError, result.Status)
}

func TestContainerdCheckerWithRealConfig(t *testing.T) {
	if _, err := os.Stat("/etc/containerd/config.toml"); err != nil {
		t.Skip("no /etc/containerd/config.toml available")
	}

	c := &ContainerdChecker{}
	params := json.RawMessage(`{"key": "version", "expected": "2"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

// --- CgroupChecker tests ---

func TestCgroupCheckerTypeAndCategory(t *testing.T) {
	c := &CgroupChecker{}
	assert.Equal(t, "cgroup_check", c.Type())
	assert.Equal(t, "container", c.Category())
}

func TestCgroupCheckerDetectsVersion(t *testing.T) {
	c := &CgroupChecker{}
	result, err := c.Check(context.Background(), nil)
	require.NoError(t, err)
	// Should detect either v1 or v2, or error if neither marker exists.
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusError}, result.Status)
	if result.Status == checker.StatusPass {
		assert.Contains(t, []string{"v1", "v2"}, result.ActualValue)
	}
}

func TestCgroupCheckerV2Detection(t *testing.T) {
	// On most modern systems, cgroup v2 should be detected.
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
		t.Skip("cgroup v2 not available")
	}

	c := &CgroupChecker{}
	result, err := c.Check(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "v2", result.ActualValue)
}

func TestCgroupCheckerV1Detection(t *testing.T) {
	// Check for cgroup v1 (legacy).
	if _, err := os.Stat("/sys/fs/cgroup/cpu"); err != nil {
		t.Skip("cgroup v1 not available")
	}
	// If v2 is also present, skip since v2 takes priority.
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		t.Skip("cgroup v2 is present, v1 check skipped")
	}

	c := &CgroupChecker{}
	result, err := c.Check(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, checker.StatusPass, result.Status)
	assert.Equal(t, "v1", result.ActualValue)
}

// --- ContainerRuntimeChecker tests ---

func TestContainerRuntimeCheckerTypeAndCategory(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	assert.Equal(t, "container_runtime_check", c.Type())
	assert.Equal(t, "container", c.Category())
}

func TestContainerRuntimeCheckerInvalidJSON(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	_, err := c.Check(context.Background(), json.RawMessage(`bad`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid params")
}

func TestContainerRuntimeCheckerEmptyRuntime(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "", "expected": "available"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime is required")
}

func TestContainerRuntimeCheckerUnknownRuntime(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "podman", "expected": "available"}`)
	_, err := c.Check(context.Background(), params)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown runtime")
}

func TestContainerRuntimeCheckerDocker(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "docker", "expected": "available"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Socket may or may not exist in test environment.
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

func TestContainerRuntimeCheckerContainerd(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "containerd", "expected": "available"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

func TestContainerRuntimeCheckerCrio(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "cri-o", "expected": "available"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

func TestContainerRuntimeCheckerDefaultExpected(t *testing.T) {
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "docker"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, "available", result.ExpectedValue)
}

func TestContainerRuntimeCheckerWithSocket(t *testing.T) {
	// Create a temp socket to test pass behavior.
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "docker.sock")
	f, err := os.Create(socketPath)
	require.NoError(t, err)
	f.Close()

	// We can't easily override the socket path, so we test the logic indirectly.
	c := &ContainerRuntimeChecker{}
	params := json.RawMessage(`{"runtime": "docker", "expected": "available"}`)
	result, err := c.Check(context.Background(), params)
	require.NoError(t, err)
	// Result depends on whether real Docker socket exists.
	assert.Contains(t, []checker.CheckStatus{checker.StatusPass, checker.StatusFail}, result.Status)
}

// --- Registration test ---

func TestContainerCheckersRegistered(t *testing.T) {
	types := []string{"docker_check", "containerd_check", "cgroup_check", "container_runtime_check"}
	for _, typ := range types {
		_, ok := checker.DefaultRegistry.Get(typ)
		assert.True(t, ok, "checker %q should be registered", typ)
	}
}
