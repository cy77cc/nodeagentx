package tunnel

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

type mockSender struct {
	mu        sync.Mutex
	dataMsgs  []struct{ id string; data []byte }
	closeMsgs []struct{ id, reason string }
}

func (m *mockSender) SendTunnelData(id string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	data := make([]byte, len(payload))
	copy(data, payload)
	m.dataMsgs = append(m.dataMsgs, struct {
		id   string
		data []byte
	}{id, data})
	return nil
}

func (m *mockSender) SendTunnelClose(id, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeMsgs = append(m.closeMsgs, struct {
		id, reason string
	}{id, reason})
	return nil
}

func (m *mockSender) DataCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dataMsgs)
}

func TestTunnelRelay(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	sender := &mockSender{}
	tun, err := NewTunnel("test-1", server, sender, 5*time.Second, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tun.Relay(ctx)

	// Write data from the "C agent" side.
	go func() {
		client.Write([]byte("hello"))
		time.Sleep(50 * time.Millisecond)
		client.Write([]byte("world"))
		time.Sleep(50 * time.Millisecond)
		cancel() // end relay
	}()

	// Wait for relay to finish.
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond)

	if sender.DataCount() < 1 {
		t.Fatal("expected at least 1 data message")
	}
}

func TestTunnelSendToTarget(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	sender := &mockSender{}
	tun, err := NewTunnel("test-2", server, sender, 5*time.Second, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 1024)
		n, _ := client.Read(buf)
		if string(buf[:n]) != "from-platform" {
			t.Errorf("expected 'from-platform', got %q", string(buf[:n]))
		}
	}()

	if err := tun.SendToTarget([]byte("from-platform")); err != nil {
		t.Fatal(err)
	}
	<-done
}

func TestTunnelIsIdle(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	sender := &mockSender{}
	tun, err := NewTunnel("test-3", server, sender, 5*time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	if tun.IsIdle() {
		t.Fatal("should not be idle immediately")
	}

	tun.lastActivity = time.Now().Add(-200 * time.Millisecond)
	if !tun.IsIdle() {
		t.Fatal("should be idle after timeout")
	}
}
