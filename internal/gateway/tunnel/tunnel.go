package tunnel

import (
	"sync"
	"time"
)

// Tunnel is a placeholder type for pool tests. Full implementation in Task 5.
type Tunnel struct {
	id           string
	mu           sync.Mutex
	lastActivity time.Time
	closed       bool
}

func (t *Tunnel) ID() string { return t.id }

// IsIdle reports whether the tunnel has been idle beyond the placeholder threshold.
func (t *Tunnel) IsIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return time.Since(t.lastActivity) > time.Minute // placeholder
}

// Close marks the tunnel as closed.
func (t *Tunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}
