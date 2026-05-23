package tunnel

import (
	"sync"
)

// Pool manages active tunnels with a maximum limit.
type Pool struct {
	mu       sync.RWMutex
	tunnels  map[string]*Tunnel
	maxCount int
}

// NewPool creates a Pool with the given maximum tunnel count.
func NewPool(maxCount int) *Pool {
	return &Pool{
		tunnels:  make(map[string]*Tunnel),
		maxCount: maxCount,
	}
}

// Add inserts a tunnel. Returns false if at capacity.
func (p *Pool) Add(t *Tunnel) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tunnels) >= p.maxCount {
		return false
	}
	p.tunnels[t.ID()] = t
	return true
}

// Get returns the tunnel with the given ID, or nil.
func (p *Pool) Get(id string) *Tunnel {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tunnels[id]
}

// Remove removes and returns the tunnel with the given ID, or nil.
func (p *Pool) Remove(id string) *Tunnel {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.tunnels[id]
	delete(p.tunnels, id)
	return t
}

// ActiveCount returns the number of active tunnels.
func (p *Pool) ActiveCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tunnels)
}

// CloseIdle closes tunnels that have been idle beyond their timeout.
func (p *Pool) CloseIdle() {
	p.mu.Lock()
	var toClose []*Tunnel
	for id, t := range p.tunnels {
		if t.IsIdle() {
			toClose = append(toClose, t)
			delete(p.tunnels, id)
		}
	}
	p.mu.Unlock()

	for _, t := range toClose {
		t.Close()
	}
}

// CloseAll closes all tunnels.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	tunnels := make([]*Tunnel, 0, len(p.tunnels))
	for _, t := range p.tunnels {
		tunnels = append(tunnels, t)
	}
	p.tunnels = make(map[string]*Tunnel)
	p.mu.Unlock()

	for _, t := range tunnels {
		t.Close()
	}
}
