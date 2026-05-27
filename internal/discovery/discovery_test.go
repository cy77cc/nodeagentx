package discovery

import (
	"context"
	"fmt"
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
	require.Len(t, lastResults, 1)
	assert.Equal(t, "redis", lastResults[0].Name)

	// Mutate the returned slice and verify internal state is unchanged
	lastResults[0].Name = "mutated"
	lastResults[0].PID = 9999

	originalResults := svc.LastResults()
	require.Len(t, originalResults, 1)
	assert.Equal(t, "redis", originalResults[0].Name, "internal state should not be affected by mutating returned slice")
	assert.Equal(t, 5678, originalResults[0].PID, "internal state should not be affected by mutating returned slice")
}

func TestLayerErrorHandling(t *testing.T) {
	goodLayer := &mockLayer{
		name: "good-layer",
		services: []Service{
			{Name: "nginx", Type: "systemd", PID: 100},
		},
	}
	badLayer := &mockLayer{
		name: "bad-layer",
		err:      fmt.Errorf("layer failure"),
		services: []Service{
			{Name: "ghost", Type: "systemd", PID: 999},
		},
	}

	cfg := Config{
		Interval: 100 * time.Millisecond,
		Layers:   []DiscoveryLayer{goodLayer, badLayer},
		Logger:   zerolog.Nop(),
	}

	svc := NewDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	results := svc.Run(ctx)

	// Discovery should continue despite the bad layer
	require.Len(t, results, 1, "should contain only the good layer's services")
	assert.Equal(t, "nginx", results[0].Name)
	assert.Equal(t, 100, results[0].PID)
}

func TestDeduplication(t *testing.T) {
	layer1 := &mockLayer{
		name: "layer-1",
		services: []Service{
			{Name: "nginx", Type: "systemd", PID: 100},
		},
	}
	layer2 := &mockLayer{
		name: "layer-2",
		services: []Service{
			{Name: "nginx", Type: "systemd", PID: 200},
		},
	}

	cfg := Config{
		Interval: 100 * time.Millisecond,
		Layers:   []DiscoveryLayer{layer1, layer2},
		Logger:   zerolog.Nop(),
	}

	svc := NewDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	results := svc.Run(ctx)

	// Should deduplicate by type:name key, keeping the first occurrence
	require.Len(t, results, 1, "duplicate type:name should be deduplicated")
	assert.Equal(t, "nginx", results[0].Name)
	assert.Equal(t, 100, results[0].PID, "should keep the first layer's service")
}

func TestNonPeriodicMode(t *testing.T) {
	layer := &mockLayer{
		name: "test-layer",
		services: []Service{
			{Name: "redis", Type: "systemd", PID: 5678},
		},
	}

	cfg := Config{
		Interval: 0, // non-periodic
		Layers:   []DiscoveryLayer{layer},
		Logger:   zerolog.Nop(),
	}

	svc := NewDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	results := svc.Run(ctx)
	elapsed := time.Since(start)

	// Should have executed discovery once
	require.Len(t, results, 1)
	assert.Equal(t, "redis", results[0].Name)

	// Should block until context is cancelled (approximately 200ms)
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(150), "Run should block until context is done")
}
