package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"

	"github.com/cy77cc/opsagent/internal/health"
)

// Sender sends proxy responses to the platform.
type Sender interface {
	SendProxyRegister(hostID, hostname, ip string, capabilities []string) error
	SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error
	SendProxyMetrics(hostID string, metrics []byte) error
}

// SSHExecutor executes commands over an SSH connection.
type SSHExecutor interface {
	Connect(ctx context.Context, addr string) (*ssh.Client, error)
	Execute(ctx context.Context, client *ssh.Client, command string, args []string) (exitCode int, stdout, stderr []byte, timedOut bool)
}

// HostConfig defines a proxy host.
type HostConfig struct {
	ID   string
	Addr string
	SSH  SSHConfig
}

// Manager handles proxy mode: executing commands on behalf of internal hosts.
type Manager struct {
	hosts      map[string]HostConfig
	sender     Sender
	logger     zerolog.Logger
	sshClients map[string]SSHExecutor
}

// NewManager creates a proxy Manager.
func NewManager(hosts []HostConfig, sender Sender, logger zerolog.Logger) *Manager {
	m := &Manager{
		hosts:      make(map[string]HostConfig),
		sender:     sender,
		logger:     logger.With().Str("component", "proxy").Logger(),
		sshClients: make(map[string]SSHExecutor),
	}
	for _, h := range hosts {
		m.hosts[h.ID] = h
		m.sshClients[h.ID] = NewSSHClient(h.SSH)
	}
	return m
}

// RegisterHosts sends proxy host registrations to the platform.
func (m *Manager) RegisterHosts() error {
	for _, h := range m.hosts {
		if err := m.sender.SendProxyRegister(h.ID, h.ID, h.Addr, nil); err != nil {
			m.logger.Warn().Err(err).Str("host_id", h.ID).Msg("failed to register proxy host")
			return err
		}
		m.logger.Info().Str("host_id", h.ID).Str("addr", h.Addr).Msg("proxy host registered")
	}
	return nil
}

// ExecuteCommand runs a command on a proxy host via SSH.
func (m *Manager) ExecuteCommand(ctx context.Context, hostID, command string, args []string, timeoutSec int32) error {
	host, ok := m.hosts[hostID]
	if !ok {
		return fmt.Errorf("proxy host %s not configured", hostID)
	}

	client, ok := m.sshClients[hostID]
	if !ok {
		return fmt.Errorf("ssh client for %s not found", hostID)
	}

	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	m.logger.Info().Str("host_id", hostID).Str("command", command).Msg("executing proxy command")
	start := time.Now()

	sshConn, err := client.Connect(ctx, host.Addr)
	if err != nil {
		m.logger.Error().Err(err).Str("host_id", hostID).Msg("ssh connect failed")
		return m.sender.SendProxyResponse(hostID, command, -1, nil, []byte(err.Error()), time.Since(start), false)
	}
	defer sshConn.Close()

	exitCode, stdout, stderr, timedOut := client.Execute(ctx, sshConn, command, args)
	duration := time.Since(start)

	m.logger.Info().Str("host_id", hostID).Int("exit_code", exitCode).Dur("duration", duration).Msg("proxy command completed")
	return m.sender.SendProxyResponse(hostID, command, exitCode, stdout, stderr, duration, timedOut)
}

// HealthStatus reports proxy manager health.
func (m *Manager) HealthStatus() health.Status {
	return health.Status{
		Status: "running",
		Details: map[string]any{
			"proxy_hosts": len(m.hosts),
		},
	}
}

// ExecuteMetricsCollect collects metrics from a proxy host via SSH.
func (m *Manager) ExecuteMetricsCollect(ctx context.Context, hostID string) error {
	host, ok := m.hosts[hostID]
	if !ok {
		return fmt.Errorf("proxy host %s not configured", hostID)
	}

	client, ok := m.sshClients[hostID]
	if !ok {
		return fmt.Errorf("ssh client for %s not found", hostID)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	sshConn, err := client.Connect(ctx, host.Addr)
	if err != nil {
		return err
	}
	defer sshConn.Close()

	return m.collectAndSendMetrics(ctx, hostID, client, sshConn)
}

// collectAndSendMetrics executes metric commands over SSH and sends the results.
func (m *Manager) collectAndSendMetrics(ctx context.Context, hostID string, client SSHExecutor, sshConn *ssh.Client) error {
	commands := map[string]string{
		"cpu":    "top -bn1 | head -5",
		"memory": "free -b",
		"disk":   "df -B1",
		"load":   "cat /proc/loadavg",
	}

	metrics := make(map[string]string)
	for name, cmd := range commands {
		_, stdout, stderr, _ := client.Execute(ctx, sshConn, cmd, nil)
		if len(stderr) > 0 {
			m.logger.Warn().Str("host_id", hostID).Str("metric", name).Bytes("stderr", stderr).Msg("metric collection command failed")
		}
		metrics[name] = string(stdout)
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal metrics for host %s: %w", hostID, err)
	}
	return m.sender.SendProxyMetrics(hostID, data)
}
