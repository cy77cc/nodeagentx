package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Service represents a discovered service on the host.
type Service struct {
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	PID          int            `json:"pid,omitempty"`
	Ports        []int          `json:"ports,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Metadata     map[string]any    `json:"metadata,omitempty"`
	DiscoveredAt time.Time      `json:"discovered_at"`
}

// DiscoveryLayer is the interface that all discovery layers must implement.
type DiscoveryLayer interface {
	Name() string
	Discover(ctx context.Context) ([]Service, error)
}

// Config holds the configuration for the DiscoveryService.
type Config struct {
	Interval time.Duration
	Layers   []DiscoveryLayer
	Logger   zerolog.Logger
}

// DiscoveryService runs discovery layers periodically and caches results.
type DiscoveryService struct {
	cfg      Config
	mu       sync.RWMutex
	lastRun  []Service
}

// NewDiscoveryService creates a new DiscoveryService with the given config.
func NewDiscoveryService(cfg Config) *DiscoveryService {
	return &DiscoveryService{
		cfg: cfg,
	}
}

// Run executes discovery immediately, then periodically on the configured interval.
// It blocks until ctx is cancelled and returns the last discovered results.
func (d *DiscoveryService) Run(ctx context.Context) []Service {
	// Run discovery immediately
	results := d.discover(ctx)
	d.mu.Lock()
	d.lastRun = results
	d.mu.Unlock()

	if d.cfg.Interval <= 0 {
		// No periodic execution; block until context is done
		<-ctx.Done()
		return d.LastResults()
	}

	ticker := time.NewTicker(d.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return d.LastResults()
		case <-ticker.C:
			results := d.discover(ctx)
			d.mu.Lock()
			d.lastRun = results
			d.mu.Unlock()
		}
	}
}

// LastResults returns a copy of the last discovery results.
func (d *DiscoveryService) LastResults() []Service {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.lastRun) == 0 {
		return nil
	}

	out := make([]Service, len(d.lastRun))
	copy(out, d.lastRun)
	return out
}

// discover iterates all layers, collects services, deduplicates by type:name key,
// and sets DiscoveredAt on each service.
func (d *DiscoveryService) discover(ctx context.Context) []Service {
	seen := make(map[string]struct{})
	var results []Service
	now := time.Now()

	for _, layer := range d.cfg.Layers {
		services, err := layer.Discover(ctx)
		if err != nil {
			d.cfg.Logger.Error().Err(err).
				Str("layer", layer.Name()).
				Msg("discovery layer failed")
			continue
		}

		for _, svc := range services {
			key := dedupKey(svc)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			svc.DiscoveredAt = now
			results = append(results, svc)
		}
	}

	return results
}

// dedupKey returns a unique key for a service based on its type and name.
func dedupKey(s Service) string {
	return fmt.Sprintf("%s:%s", s.Type, s.Name)
}
