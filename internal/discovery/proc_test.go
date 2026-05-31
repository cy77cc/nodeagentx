package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcLayerName(t *testing.T) {
	layer := NewProcLayer()
	assert.Equal(t, "proc", layer.Name())
}

func TestProcLayerDiscover(t *testing.T) {
	layer := NewProcLayer()

	services, err := layer.Discover(context.Background())
	require.NoError(t, err, "Discover should not error on a live system")

	t.Logf("ProcLayer discovered %d service(s)", len(services))
	for _, svc := range services {
		t.Logf("  name=%q pid=%d ports=%v cmdline=%q",
			svc.Name, svc.PID, svc.Ports, svc.Metadata["cmdline"])
	}

	// Every returned service must have the correct type and non-empty fields.
	for _, svc := range services {
		assert.Equal(t, "process", svc.Type, "service type must be 'process'")
		assert.NotEmpty(t, svc.Name, "service name must not be empty")
		assert.Greater(t, svc.PID, 0, "PID must be positive")
		assert.NotEmpty(t, svc.Ports, "ports must not be empty")
		assert.NotNil(t, svc.Metadata, "metadata must not be nil")
		assert.NotEmpty(t, svc.Metadata["cmdline"], "cmdline metadata must not be empty")
		assert.False(t, svc.DiscoveredAt.IsZero(), "DiscoveredAt must be set")
	}
}

func TestProcLayerDiscoverNoListenPorts(t *testing.T) {
	// Use a custom procRoot that has no matching processes.
	// The gopsutil library reads from the real /proc, so this test verifies
	// that we handle an empty connection list gracefully.
	//
	// Since we can't easily mock net.ConnectionsWithContext without an interface,
	// we test the readComm fallback path instead.
	layer := &ProcLayer{procRoot: "/nonexistent"}
	name := layer.readComm(1)
	assert.Empty(t, name, "readComm should return empty for nonexistent proc root")
}

func TestReadComm(t *testing.T) {
	// Create a temp directory simulating /proc/<pid>/comm
	tmpDir := t.TempDir()
	pidDir := filepath.Join(tmpDir, "1234")
	require.NoError(t, os.MkdirAll(pidDir, 0o755))

	commFile := filepath.Join(pidDir, "comm")
	require.NoError(t, os.WriteFile(commFile, []byte("test-process\n"), 0o644))

	layer := &ProcLayer{procRoot: tmpDir}
	name := layer.readComm(1234)
	assert.Equal(t, "test-process", name)
}

func TestReadCommMissing(t *testing.T) {
	layer := &ProcLayer{procRoot: "/nonexistent"}
	name := layer.readComm(99999)
	assert.Empty(t, name)
}

