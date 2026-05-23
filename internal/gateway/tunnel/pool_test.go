package tunnel

import (
	"testing"
	"time"
)

func TestPoolAdd(t *testing.T) {
	p := NewPool(2)
	t1 := &Tunnel{id: "a", lastActivity: time.Now()}
	t2 := &Tunnel{id: "b", lastActivity: time.Now()}
	t3 := &Tunnel{id: "c", lastActivity: time.Now()}

	if !p.Add(t1) {
		t.Fatal("expected add t1 to succeed")
	}
	if !p.Add(t2) {
		t.Fatal("expected add t2 to succeed")
	}
	if p.Add(t3) {
		t.Fatal("expected add t3 to fail (at capacity)")
	}
}

func TestPoolGet(t *testing.T) {
	p := NewPool(10)
	t1 := &Tunnel{id: "x", lastActivity: time.Now()}
	p.Add(t1)

	if got := p.Get("x"); got != t1 {
		t.Fatalf("expected t1, got %v", got)
	}
	if got := p.Get("missing"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestPoolRemove(t *testing.T) {
	p := NewPool(10)
	t1 := &Tunnel{id: "x", lastActivity: time.Now()}
	p.Add(t1)

	removed := p.Remove("x")
	if removed != t1 {
		t.Fatalf("expected t1, got %v", removed)
	}
	if p.Get("x") != nil {
		t.Fatal("expected tunnel to be removed")
	}
	if p.ActiveCount() != 0 {
		t.Fatalf("expected 0, got %d", p.ActiveCount())
	}
}

func TestPoolActiveCount(t *testing.T) {
	p := NewPool(10)
	p.Add(&Tunnel{id: "a", lastActivity: time.Now()})
	p.Add(&Tunnel{id: "b", lastActivity: time.Now()})
	if p.ActiveCount() != 2 {
		t.Fatalf("expected 2, got %d", p.ActiveCount())
	}
}

func TestPoolCloseAll(t *testing.T) {
	p := NewPool(10)
	p.Add(&Tunnel{id: "a", lastActivity: time.Now()})
	p.Add(&Tunnel{id: "b", lastActivity: time.Now()})
	p.CloseAll()
	if p.ActiveCount() != 0 {
		t.Fatalf("expected 0 after CloseAll, got %d", p.ActiveCount())
	}
}

func TestPoolCloseIdle(t *testing.T) {
	p := NewPool(10)
	t1 := &Tunnel{id: "idle", lastActivity: time.Now().Add(-10 * time.Minute), idleTimeout: time.Minute}
	t2 := &Tunnel{id: "active", lastActivity: time.Now(), idleTimeout: time.Minute}
	p.Add(t1)
	p.Add(t2)

	p.CloseIdle()

	if p.Get("idle") != nil {
		t.Fatal("expected idle tunnel to be closed")
	}
	if p.Get("active") == nil {
		t.Fatal("expected active tunnel to remain")
	}
}
