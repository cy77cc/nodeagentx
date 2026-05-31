package gateway

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/cy77cc/opsagent/internal/gateway/proxy"
	"github.com/cy77cc/opsagent/internal/gateway/tunnel"
	"github.com/cy77cc/opsagent/internal/health"
)

// TunnelSender sends tunnel messages to the platform via gRPC.
type TunnelSender interface {
	SendTunnelOpen(tunnelID, agentID, hostname, ip string, capabilities []string) error
	SendTunnelData(tunnelID string, payload []byte) error
	SendTunnelClose(tunnelID, reason string) error
}

// ProxySender sends proxy responses to the platform via gRPC.
type ProxySender interface {
	SendProxyRegister(hostID, hostname, ip string, capabilities []string) error
	SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error
	SendProxyMetrics(hostID string, metrics []byte) error
}

// Gateway manages tunnel and proxy subsystems for jump-host functionality.
type Gateway struct {
	cfg    Config
	logger zerolog.Logger

	tunnelSender TunnelSender
	proxySender  ProxySender

	listener net.Listener
	pool     *tunnel.Pool

	mu sync.RWMutex
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// New creates a Gateway. Call Start to begin accepting connections.
func New(cfg Config, logger zerolog.Logger, tunnelSender TunnelSender, proxySender ProxySender) (*Gateway, error) {
	if tunnelSender == nil {
		return nil, fmt.Errorf("gateway: tunnelSender must not be nil")
	}
	if proxySender == nil {
		return nil, fmt.Errorf("gateway: proxySender must not be nil")
	}
	return &Gateway{
		cfg:          cfg,
		logger:       logger.With().Str("component", "gateway").Logger(),
		tunnelSender: tunnelSender,
		proxySender:  proxySender,
		pool:         tunnel.NewPool(cfg.MaxTunnels),
	}, nil
}

// Start begins the gateway listener and background routines.
func (g *Gateway) Start(ctx context.Context) error {
	g.ctx, g.cancel = context.WithCancel(ctx)

	if g.cfg.ListenAddr != "" {
		ln, err := net.Listen("tcp", g.cfg.ListenAddr)
		if err != nil {
			return fmt.Errorf("gateway listen %s: %w", g.cfg.ListenAddr, err)
		}
		g.listener = ln
		g.logger.Info().Str("addr", g.cfg.ListenAddr).Msg("gateway listener started")

		g.wg.Go(func() {
			g.acceptLoop()
		})
	}

	g.wg.Go(func() {
		g.idleReaper()
	})

	// Register proxy hosts.
	for _, h := range g.cfg.Hosts {
		if h.Mode == "proxy" || h.Mode == "auto" {
			hostname := h.Hostname
			if hostname == "" {
				hostname = h.ID
			}
			if err := g.proxySender.SendProxyRegister(h.ID, hostname, h.Addr, nil); err != nil {
				g.logger.Warn().Err(err).Str("host_id", h.ID).Msg("failed to register proxy host")
			}
		}
	}

	g.mu.Lock()
	g.running = true
	g.mu.Unlock()

	g.logger.Info().Int("hosts", len(g.cfg.Hosts)).Msg("gateway started")
	return nil
}

// Stop gracefully shuts down the gateway.
func (g *Gateway) Stop(ctx context.Context) error {
	g.mu.Lock()
	if !g.running {
		g.mu.Unlock()
		return nil
	}
	g.running = false
	g.mu.Unlock()

	if g.cancel != nil {
		g.cancel()
	}

	if g.listener != nil {
		g.listener.Close()
	}

	g.pool.CloseAll()

	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	g.logger.Info().Msg("gateway stopped")
	return nil
}

// HealthStatus reports gateway health.
func (g *Gateway) HealthStatus() health.Status {
	g.mu.RLock()
	running := g.running
	g.mu.RUnlock()

	if !running {
		return health.Status{Status: "stopped"}
	}

	active := g.pool.ActiveCount()
	return health.Status{
		Status: "running",
		Details: map[string]any{
			"active_tunnels": active,
			"max_tunnels":    g.cfg.MaxTunnels,
		},
	}
}

// HandleTunnelData processes tunnel data from the platform.
func (g *Gateway) HandleTunnelData(tunnelID string, data []byte) error {
	t := g.pool.Get(tunnelID)
	if t == nil {
		return fmt.Errorf("tunnel %s not found", tunnelID)
	}
	return t.SendToTarget(data)
}

// HandleTunnelClose processes tunnel close from the platform.
func (g *Gateway) HandleTunnelClose(tunnelID, reason string) error {
	t := g.pool.Remove(tunnelID)
	if t == nil {
		return nil
	}
	g.logger.Info().Str("tunnel_id", tunnelID).Str("reason", reason).Msg("tunnel closed by platform")
	return t.Close()
}

// HandleProxyCommand processes a proxy command from the platform.
func (g *Gateway) HandleProxyCommand(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error {
	var hostCfg *HostConfig
	for _, h := range g.cfg.Hosts {
		if h.ID == hostID {
			hostCfg = &h
			break
		}
	}
	if hostCfg == nil {
		return fmt.Errorf("proxy host %s not configured", hostID)
	}

	g.logger.Info().Str("host_id", hostID).Str("command", command).Msg("proxy command received")

	start := time.Now()
	exitCode, stdout, stderr, timedOut := g.executeProxyCommand(ctx, *hostCfg, command, args, timeoutSec)
	duration := time.Since(start)

	return g.proxySender.SendProxyResponse(hostID, command, exitCode, stdout, stderr, duration, timedOut)
}

func (g *Gateway) acceptLoop() {
	for {
		conn, err := g.listener.Accept()
		if err != nil {
			select {
			case <-g.ctx.Done():
				return
			default:
				g.logger.Error().Err(err).Msg("gateway accept error")
				continue
			}
		}
		g.wg.Go(func() {
			g.handleIncoming(conn)
		})
	}
}

func (g *Gateway) handleIncoming(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()

	// Authenticate if PSK is configured.
	if g.cfg.AuthPSK != "" {
		if err := g.authenticateConnection(conn); err != nil {
			g.logger.Warn().Str("remote", remoteAddr).Err(err).Msg("tunnel auth failed")
			return
		}
	} else {
		g.logger.Warn().Str("remote", remoteAddr).Msg("tunnel auth disabled (no PSK configured)")
	}

	g.logger.Info().Str("remote", remoteAddr).Msg("incoming connection")

	// Read initial bytes to determine if this is an agent connection.
	// For now, treat all incoming connections as tunnel candidates.
	// Generate unpredictable tunnel ID.
	randBytes := make([]byte, 8)
	if _, err := rand.Read(randBytes); err != nil {
		g.logger.Error().Err(err).Msg("failed to generate tunnel ID")
		return
	}
	tunnelID := "tunnel-" + hex.EncodeToString(randBytes)

	t, err := tunnel.NewTunnel(tunnelID, conn, g.tunnelSender, g.cfg.TunnelTimeout, g.cfg.IdleTimeout)
	if err != nil {
		g.logger.Error().Err(err).Str("remote", remoteAddr).Msg("failed to create tunnel")
		return
	}

	if !g.pool.Add(t) {
		g.logger.Warn().Str("remote", remoteAddr).Msg("max tunnels reached, rejecting")
		t.Close()
		return
	}

	g.logger.Info().Str("tunnel_id", tunnelID).Str("remote", remoteAddr).Msg("tunnel created")

	// Notify platform.
	if err := g.tunnelSender.SendTunnelOpen(tunnelID, "", "", remoteAddr, nil); err != nil {
		g.logger.Error().Err(err).Msg("failed to send tunnel open")
		g.pool.Remove(tunnelID)
		t.Close()
		return
	}

	// Run relay until tunnel closes.
	t.Relay(g.ctx)
	g.pool.Remove(tunnelID)
	g.logger.Info().Str("tunnel_id", tunnelID).Msg("tunnel relay ended")
}

func (g *Gateway) authenticateConnection(conn net.Conn) error {
	buf := make([]byte, 32)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return fmt.Errorf("read auth token: %w", err)
	}
	if subtle.ConstantTimeCompare(buf, []byte(g.cfg.AuthPSK)) != 1 {
		return fmt.Errorf("invalid auth token")
	}
	return nil
}

func (g *Gateway) idleReaper() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-g.ctx.Done():
			return
		case <-ticker.C:
			g.pool.CloseIdle()
		}
	}
}

func (g *Gateway) executeProxyCommand(ctx context.Context, host HostConfig, command string, args []string, timeoutSec int32) (int, []byte, []byte, bool) {
	sshClient := proxy.NewSSHClient(proxy.SSHConfig{
		User:     host.SSH.User,
		Password: host.SSH.Password,
		KeyFile:  host.SSH.KeyFile,
		Port:     host.SSH.Port,
	})

	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := sshClient.Connect(ctx, host.Addr)
	if err != nil {
		g.logger.Error().Err(err).Str("host_id", host.ID).Msg("ssh connect failed")
		return -1, nil, []byte(err.Error()), false
	}
	defer conn.Close()

	exitCode, stdout, stderr, timedOut := sshClient.Execute(ctx, conn, command, args)
	return exitCode, stdout, stderr, timedOut
}
