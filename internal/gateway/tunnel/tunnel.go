package tunnel

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
)

// Sender sends tunnel data to the platform.
type Sender interface {
	SendTunnelData(tunnelID string, payload []byte) error
	SendTunnelClose(tunnelID, reason string) error
}

// Tunnel bridges a TCP connection with the platform via gRPC tunnel messages.
type Tunnel struct {
	id            string
	conn          net.Conn
	sender        Sender
	tunnelTimeout time.Duration
	idleTimeout   time.Duration

	mu           sync.Mutex
	lastActivity time.Time
	closed       bool
	done         chan struct{}
}

// NewTunnel creates a Tunnel wrapping the given TCP connection.
func NewTunnel(id string, conn net.Conn, sender Sender, tunnelTimeout, idleTimeout time.Duration) (*Tunnel, error) {
	return &Tunnel{
		id:            id,
		conn:          conn,
		sender:        sender,
		tunnelTimeout: tunnelTimeout,
		idleTimeout:   idleTimeout,
		lastActivity:  time.Now(),
		done:          make(chan struct{}),
	}, nil
}

// ID returns the tunnel identifier.
func (t *Tunnel) ID() string { return t.id }

// IsIdle reports whether the tunnel has exceeded its idle timeout.
func (t *Tunnel) IsIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.lastActivity) > t.idleTimeout
}

// SendToTarget writes data from the platform to the TCP connection.
func (t *Tunnel) SendToTarget(data []byte) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return io.ErrClosedPipe
	}
	t.lastActivity = time.Now()
	t.mu.Unlock()

	t.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := t.conn.Write(data)
	return err
}

// Close shuts down the tunnel.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	if t.done != nil {
		close(t.done)
	}
	t.mu.Unlock()

	if t.sender != nil {
		t.sender.SendTunnelClose(t.id, "closed")
	}
	if t.conn != nil {
		return t.conn.Close()
	}
	return nil
}

// Relay reads from the TCP connection and sends to platform until context cancelled or connection closed.
func (t *Tunnel) Relay(ctx context.Context) {
	defer t.conn.Close()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			t.Close()
			return
		case <-t.done:
			return
		default:
		}

		t.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := t.conn.Read(buf)
		if n > 0 {
			t.mu.Lock()
			t.lastActivity = time.Now()
			t.mu.Unlock()

			payload := make([]byte, n)
			copy(payload, buf[:n])

			if sendErr := t.sender.SendTunnelData(t.id, payload); sendErr != nil {
				t.Close()
				return
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // read timeout, loop back to check context
			}
			if err != io.EOF {
				t.Close()
			}
			return
		}
	}
}
