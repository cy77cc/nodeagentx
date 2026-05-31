package syslog

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

// waitForMetrics polls acc.Collect() until at least minCount metrics have been
// accumulated (across all Collect calls) or the timeout elapses. Collect()
// drains the buffer, so results from successive calls are merged.
func waitForMetrics(t *testing.T, acc collector.Accumulator, minCount int) []*collector.Metric {
	t.Helper()
	var all []*collector.Metric
	deadline := time.After(2 * time.Second)
	for {
		metrics := acc.Collect()
		all = append(all, metrics...)
		if len(all) >= minCount {
			return all
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d metrics, got %d", minCount, len(all))
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSyslogInputInit(t *testing.T) {
	si := &SyslogInput{}
	cfg := map[string]any{
		"listen_addr":     "127.0.0.1:1514",
		"protocol":        "udp",
		"max_connections": 50,
	}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if si.ListenAddr != "127.0.0.1:1514" {
		t.Errorf("expected listen_addr=127.0.0.1:1514, got %s", si.ListenAddr)
	}
	if si.Protocol != "udp" {
		t.Errorf("expected protocol=udp, got %s", si.Protocol)
	}
	if si.MaxConnections != 50 {
		t.Errorf("expected max_connections=50, got %d", si.MaxConnections)
	}
}

func TestSyslogInputInitDefaults(t *testing.T) {
	si := &SyslogInput{}
	cfg := map[string]any{}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if si.ListenAddr != "0.0.0.0:514" {
		t.Errorf("expected default listen_addr=0.0.0.0:514, got %s", si.ListenAddr)
	}
	if si.Protocol != "tcp" {
		t.Errorf("expected default protocol=tcp, got %s", si.Protocol)
	}
	if si.MaxConnections != 100 {
		t.Errorf("expected default max_connections=100, got %d", si.MaxConnections)
	}
}

func TestSyslogInputGatherTCP(t *testing.T) {
	si := &SyslogInput{}
	cfg := map[string]any{
		"listen_addr": "127.0.0.1:0",
		"protocol":    "tcp",
	}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	acc := collector.NewAccumulator(100)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- si.Gather(ctx, acc)
	}()

	// Wait for listener to start (ready is created in Init, closed by gatherTCP)
	<-si.ready

	// Connect and send a syslog message
	conn, err := net.Dial("tcp", si.listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	fmt.Fprintf(conn, "<13>Jan 15 10:30:00 myhost test: hello world\n")
	conn.Close()

	// Poll for metrics instead of fixed sleep
	metrics := waitForMetrics(t, acc, 1)

	// Cancel context to stop Gather
	cancel()

	// Wait for Gather to return
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Gather returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Gather did not return after context cancellation")
	}

	// Verify metrics
	if len(metrics) < 1 {
		t.Fatalf("expected at least 1 metric, got %d", len(metrics))
	}

	m := metrics[0]
	if m.Name() != "syslog" {
		t.Errorf("expected metric name=syslog, got %s", m.Name())
	}

	tags := m.Tags()
	if tags["host"] != "myhost" {
		t.Errorf("expected tag host=myhost, got %s", tags["host"])
	}
	if tags["app"] != "test" {
		t.Errorf("expected tag app=test, got %s", tags["app"])
	}

	fields := m.Fields()
	if fields["message"] != "hello world" {
		t.Errorf("expected field message=hello world, got %v", fields["message"])
	}
	if fields["facility"] != 1 {
		t.Errorf("expected field facility=1, got %v", fields["facility"])
	}
	if fields["severity"] != 5 {
		t.Errorf("expected field severity=5, got %v", fields["severity"])
	}
}

func TestSyslogInputSampleConfig(t *testing.T) {
	si := &SyslogInput{}
	cfg := si.SampleConfig()
	if cfg == "" {
		t.Error("expected non-empty sample config")
	}
}

func TestSyslogInputGatherUDP(t *testing.T) {
	si := &SyslogInput{}
	cfg := map[string]any{
		"listen_addr": "127.0.0.1:0",
		"protocol":    "udp",
	}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	acc := collector.NewAccumulator(100)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- si.Gather(ctx, acc)
	}()

	// Wait for listener to start
	<-si.ready

	// Send a syslog message via UDP
	conn, err := net.Dial("udp", si.udpConn.LocalAddr().String())
	if err != nil {
		t.Fatalf("failed to dial UDP: %v", err)
	}
	fmt.Fprintf(conn, "<13>Jan 15 10:30:00 myhost udptest: udp hello")
	conn.Close()

	// Poll for metrics instead of fixed sleep
	metrics := waitForMetrics(t, acc, 1)

	// Cancel context to stop Gather
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Gather returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Gather did not return after context cancellation")
	}

	if len(metrics) < 1 {
		t.Fatalf("expected at least 1 metric, got %d", len(metrics))
	}

	m := metrics[0]
	if m.Name() != "syslog" {
		t.Errorf("expected metric name=syslog, got %s", m.Name())
	}
	if m.Tags()["app"] != "udptest" {
		t.Errorf("expected tag app=udptest, got %s", m.Tags()["app"])
	}
	if m.Fields()["message"] != "udp hello" {
		t.Errorf("expected field message=udp hello, got %v", m.Fields()["message"])
	}
}

func TestSyslogInputMultipleTCPClients(t *testing.T) {
	si := &SyslogInput{}
	cfg := map[string]any{
		"listen_addr":     "127.0.0.1:0",
		"protocol":        "tcp",
		"max_connections": 2,
	}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	acc := collector.NewAccumulator(100)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- si.Gather(ctx, acc)
	}()

	<-si.ready

	// Send messages from two clients
	for i := range 2 {
		conn, err := net.Dial("tcp", si.listener.Addr().String())
		if err != nil {
			t.Fatalf("client %d: failed to dial: %v", i, err)
		}
		fmt.Fprintf(conn, "<13>Jan 15 10:30:00 host%d app%d: msg %d\n", i, i, i)
		conn.Close()
	}

	// Poll for metrics instead of fixed sleep
	metrics := waitForMetrics(t, acc, 2)

	cancel()

	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Gather did not return")
	}

	if len(metrics) < 2 {
		t.Fatalf("expected at least 2 metrics, got %d", len(metrics))
	}
}
