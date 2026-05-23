package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type mockSender struct {
	registers []struct{ hostID, hostname, ip string }
	responses []struct {
		hostID, command string
		exitCode        int
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
	return nil
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
