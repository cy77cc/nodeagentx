package federation

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectAutoLabels_ReturnsOSAndArch(t *testing.T) {
	labels := CollectAutoLabels()
	require.Contains(t, labels, "os")
	require.Contains(t, labels, "arch")
	assert.Equal(t, runtime.GOOS, labels["os"])
	assert.Equal(t, runtime.GOARCH, labels["arch"])
}

func TestCollectAutoLabels_ReturnsHostname(t *testing.T) {
	labels := CollectAutoLabels()
	require.Contains(t, labels, "hostname")
	assert.NotEmpty(t, labels["hostname"])
}

func TestCollectAutoLabels_ReturnsKernelVersion(t *testing.T) {
	labels := CollectAutoLabels()
	if runtime.GOOS == "linux" {
		assert.Contains(t, labels, "kernel_version")
	}
}
