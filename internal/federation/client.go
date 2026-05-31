package federation

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type LeafClientConfig struct {
	AgentID           string
	HubAddr           string
	Labels            map[string]string
	AutoLabels        map[string]string
	ReconnectSec      int
	ReportIntervalSec int
	Logger            zerolog.Logger
}

type LeafClient struct {
	cfg       LeafClientConfig
	mu        sync.RWMutex
	connected bool
	cancel    context.CancelFunc
}

func NewLeafClient(cfg LeafClientConfig) *LeafClient {
	return &LeafClient{cfg: cfg}
}

func (lc *LeafClient) Start(ctx context.Context) error {
	ctx, lc.cancel = context.WithCancel(ctx)
	go lc.connectLoop(ctx)
	return nil
}

func (lc *LeafClient) Stop() {
	if lc.cancel != nil {
		lc.cancel()
	}
	lc.mu.Lock()
	lc.connected = false
	lc.mu.Unlock()
}

func (lc *LeafClient) IsConnected() bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return lc.connected
}

func (lc *LeafClient) HealthStatus() map[string]any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return map[string]any{
		"connected": lc.connected,
		"hub_addr":  lc.cfg.HubAddr,
	}
}

func (lc *LeafClient) connectLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := lc.connect(ctx)
		if err != nil {
			lc.cfg.Logger.Warn().Err(err).Msg("Hub connection failed, retrying")
			lc.mu.Lock()
			lc.connected = false
			lc.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(lc.cfg.ReconnectSec) * time.Second):
			}
		}
	}
}

func (lc *LeafClient) connect(ctx context.Context) error {
	lc.mu.Lock()
	lc.connected = true
	lc.mu.Unlock()
	lc.cfg.Logger.Info().Str("hub_addr", lc.cfg.HubAddr).Msg("Connected to Hub")
	<-ctx.Done()
	return ctx.Err()
}
