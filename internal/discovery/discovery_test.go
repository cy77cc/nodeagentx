package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLayer is a test double for DiscoveryLayer.
type mockLayer struct {
	name     string
	services []Service
	err      error
}

func (m *mockLayer) Name() string {
	return m.name
}

func (m *mockLayer) Discover(ctx context.Context) ([]Service, error) {
	return m.services, m.err
}

func TestDiscoveryServiceRun(t *testing.T) {
	layer := &mockLayer{
		name: "test-layer",
		services: []Service{
			{
				Name: "nginx",
				Type: "systemd",
				PID:  1234,
				Ports: []int{80, 443},
				Labels: map[string]string{
					"env": "production",
				},
				Metadata: map[string]any{
					"version": "1.25.0",
				},
			},
		},
	}

	cfg := Config{
		Interval: 100 * time.Millisecond,
		Layers:   []DiscoveryLayer{layer},
		Logger:   zerolog.Nop(),
	}

	svc := NewDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	results := svc.Run(ctx)

	require.Len(t, results, 1)
	assert.Equal(t, "nginx", results[0].Name)
	assert.Equal(t, "systemd", results[0].Type)
	assert.Equal(t, 1234, results[0].PID)
	assert.Equal(t, []int{80, 443}, results[0].Ports)
	assert.Equal(t, "production", results[0].Labels["env"])
	assert.Equal(t, "1.25.0", results[0].Metadata["version"])
	assert.False(t, results[0].DiscoveredAt.IsZero())
}

func TestDiscoveryServiceNoLayers(t *testing.T) {
	cfg := Config{
		Interval: 100 * time.Millisecond,
		Layers:   []DiscoveryLayer{},
		Logger:   zerolog.Nop(),
	}

	svc := NewDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	results := svc.Run(ctx)

	assert.Empty(t, results)
}

func TestLastResultsReturnsCopy(t *testing.T) {
	layer := &mockLayer{
		name: "test-layer",
		services: []Service{
			{
				Name: "redis",
				Type: "systemd",
				PID:  5678,
			},
		},
	}

	cfg := Config{
		Interval: 50 * time.Millisecond,
		Layers:   []DiscoveryLayer{layer},
		Logger:   zerolog.Nop(),
	}

	svc := NewDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	results := svc.Run(ctx)
	require.Len(t, results, 1)

	// LastResults should return a copy, not the same slice
	lastResults := svc.LastResults()
	assert.Len(t, lastResults, 1)
	assert.Equal(t, "redis", lastResults[0].Name)
}
