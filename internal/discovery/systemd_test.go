package discovery

import (
	"context"
	"os/exec"
	"testing"
)

func TestSystemdLayerName(t *testing.T) {
	layer := &SystemdLayer{}
	if got := layer.Name(); got != "systemd" {
		t.Errorf("Name() = %q, want %q", got, "systemd")
	}
}

func TestSystemdLayerDiscoverNoSystemctl(t *testing.T) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("systemctl not available, skipping")
	}

	layer := &SystemdLayer{}
	ctx := context.Background()
	_, err := layer.Discover(ctx)
	if err != nil {
		t.Errorf("Discover() returned unexpected error: %v", err)
	}
}
