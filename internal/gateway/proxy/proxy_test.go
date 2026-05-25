package proxy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/crypto/ssh"
)

type mockSender struct {
	registers []struct{ hostID, hostname, ip string }
	responses []struct {
		hostID, command string
		exitCode        int
	}
	metricsCalls []struct {
		hostID  string
		metrics []byte
	}
}

func (m *mockSender) SendProxyRegister(hostID, hostname, ip string, capabilities []string) error {
	m.registers = append(m.registers, struct{ hostID, hostname, ip string }{hostID, hostname, ip})
	return nil
}

func (m *mockSender) SendProxyResponse(hostID, command string, exitCode int, stdout, stderr []byte, duration time.Duration, timedOut bool) error {
	m.responses = append(m.responses, struct {
		hostID, command string
		exitCode        int
	}{hostID, command, exitCode})
	return nil
}

func (m *mockSender) SendProxyMetrics(hostID string, metrics []byte) error {
	m.metricsCalls = append(m.metricsCalls, struct {
		hostID  string
		metrics []byte
	}{hostID, metrics})
	return nil
}

type mockSSHExecutor struct {
	connectErr error
	executeFn  func(ctx context.Context, client *ssh.Client, command string, args []string) (int, []byte, []byte, bool)
}

func (m *mockSSHExecutor) Connect(ctx context.Context, addr string) (*ssh.Client, error) {
	if m.connectErr != nil {
		return nil, m.connectErr
	}
	return nil, nil // nil client is fine since executeFn doesn't use it
}

func (m *mockSSHExecutor) Execute(ctx context.Context, client *ssh.Client, command string, args []string) (int, []byte, []byte, bool) {
	if m.executeFn != nil {
		return m.executeFn(ctx, client, command, args)
	}
	return 0, []byte("mock output"), nil, false
}

func TestManagerRegisterHosts(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	hosts := []HostConfig{
		{ID: "c1", Addr: "192.168.1.10", SSH: SSHConfig{User: "root", Password: "pass", Port: 22}},
		{ID: "c2", Addr: "192.168.1.11", SSH: SSHConfig{User: "root", Password: "pass", Port: 22}},
	}

	m := NewManager(hosts, sender, logger)
	if err := m.RegisterHosts(); err != nil {
		t.Fatal(err)
	}

	if len(sender.registers) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(sender.registers))
	}
}

func TestManagerExecuteCommandUnknownHost(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	m := NewManager(nil, sender, logger)

	err := m.ExecuteCommand(context.Background(), "unknown", "uptime", nil, 10)
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
}

func TestManagerHealthStatus(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	hosts := []HostConfig{
		{ID: "c1", Addr: "192.168.1.10", SSH: SSHConfig{User: "root", Password: "pass", Port: 22}},
	}
	m := NewManager(hosts, sender, logger)

	st := m.HealthStatus()
	if st.Status != "running" {
		t.Fatalf("expected running, got %s", st.Status)
	}
}

func TestCollectAndSendMetricsSuccess(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	m := NewManager(nil, sender, logger)

	executor := &mockSSHExecutor{
		executeFn: func(_ context.Context, _ *ssh.Client, command string, _ []string) (int, []byte, []byte, bool) {
			return 0, []byte("mock " + command), nil, false
		},
	}

	err := m.collectAndSendMetrics(context.Background(), "c1", executor, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.metricsCalls) != 1 {
		t.Fatalf("expected 1 SendProxyMetrics call, got %d", len(sender.metricsCalls))
	}

	if sender.metricsCalls[0].hostID != "c1" {
		t.Errorf("expected hostID c1, got %s", sender.metricsCalls[0].hostID)
	}

	// Verify the metrics are valid JSON with expected keys.
	var metrics map[string]string
	if err := json.Unmarshal(sender.metricsCalls[0].metrics, &metrics); err != nil {
		t.Fatalf("metrics not valid JSON: %v", err)
	}
	for _, key := range []string{"cpu", "memory", "disk", "load"} {
		if _, ok := metrics[key]; !ok {
			t.Errorf("missing metric key: %s", key)
		}
	}
}

func TestExecuteMetricsCollectUnknownHost(t *testing.T) {
	sender := &mockSender{}
	logger := zerolog.Nop()
	m := NewManager(nil, sender, logger)

	err := m.ExecuteMetricsCollect(context.Background(), "unknown")
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
}
