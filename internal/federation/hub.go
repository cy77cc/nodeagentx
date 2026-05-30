package federation

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
)

type HubConfig struct {
	ListenAddr   string
	Region       string
	MaxLeaves    int
	Groups       []GroupRule
	ConfigLevels ConfigLevels
	Logger       zerolog.Logger
}

type Hub struct {
	cfg               HubConfig
	mu                sync.RWMutex
	server            *HubServer
	groupEngine       *GroupEngine
	configDistributor *ConfigDistributor
	running           bool
	cancel            context.CancelFunc
}

func NewHub(cfg HubConfig) *Hub {
	ge := NewGroupEngine(cfg.Groups)
	cd := NewConfigDistributor(cfg.ConfigLevels, ge)
	srv := NewHubServer(HubServerConfig{
		Region:            cfg.Region,
		MaxLeaves:         cfg.MaxLeaves,
		ListenAddr:        cfg.ListenAddr,
		GroupEngine:       ge,
		ConfigDistributor: cd,
		Logger:            cfg.Logger,
	})
	return &Hub{
		cfg:               cfg,
		server:            srv,
		groupEngine:       ge,
		configDistributor: cd,
	}
}

func (h *Hub) Start(ctx context.Context) error {
	ctx, h.cancel = context.WithCancel(ctx)
	h.mu.Lock()
	h.running = true
	h.mu.Unlock()
	h.cfg.Logger.Info().Str("region", h.cfg.Region).Str("listen_addr", h.cfg.ListenAddr).Msg("Hub started")
	<-ctx.Done()
	return nil
}

func (h *Hub) Stop() {
	if h.cancel != nil {
		h.cancel()
	}
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()
	h.cfg.Logger.Info().Msg("Hub stopped")
}

func (h *Hub) HealthStatus() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()
	status := "stopped"
	if h.running {
		status = "running"
	}
	leaves := h.server.GetLeaves()
	onlineCount := 0
	for _, l := range leaves {
		if l.Status == LeafStatusOnline {
			onlineCount++
		}
	}
	return map[string]interface{}{
		"status":         status,
		"region":         h.cfg.Region,
		"leaves_total":   len(leaves),
		"leaves_online":  onlineCount,
		"leaves_offline": len(leaves) - onlineCount,
	}
}

func (h *Hub) GetServer() *HubServer {
	return h.server
}
