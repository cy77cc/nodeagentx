package tracing

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchProcessor(t *testing.T) {
	var exported [][]byte
	var mu sync.Mutex

	bp := NewBatchProcessor(BatchConfig{
		MaxSize: 3,
		Timeout: 10 * time.Second,
	})
	defer bp.Stop()

	bp.SetExportFn(func(batch [][]byte) error {
		mu.Lock()
		defer mu.Unlock()
		exported = append(exported, batch...)
		return nil
	})

	bp.Add([]byte("item1"))
	bp.Add([]byte("item2"))
	bp.Add([]byte("item3"))

	// Give a moment for the flush to complete
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(exported) != 3 {
		t.Fatalf("expected 3 exported items, got %d", len(exported))
	}

	expected := []string{"item1", "item2", "item3"}
	for i, e := range expected {
		if string(exported[i]) != e {
			t.Errorf("exported[%d] = %q, want %q", i, string(exported[i]), e)
		}
	}
}

func TestBatchProcessorTimeout(t *testing.T) {
	var exported [][]byte
	var mu sync.Mutex

	bp := NewBatchProcessor(BatchConfig{
		MaxSize: 100,
		Timeout: 50 * time.Millisecond,
	})
	defer bp.Stop()

	bp.SetExportFn(func(batch [][]byte) error {
		mu.Lock()
		defer mu.Unlock()
		exported = append(exported, batch...)
		return nil
	})

	bp.Add([]byte("timeout-item"))

	// Wait for the timeout-based flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(exported) != 1 {
		t.Fatalf("expected 1 exported item from timeout flush, got %d", len(exported))
	}
	if string(exported[0]) != "timeout-item" {
		t.Errorf("exported[0] = %q, want %q", string(exported[0]), "timeout-item")
	}
}

func TestBatchProcessorStop(t *testing.T) {
	var exportCount atomic.Int32
	var mu sync.Mutex
	var exported [][]byte

	bp := NewBatchProcessor(BatchConfig{
		MaxSize: 100,
		Timeout: 10 * time.Second,
	})

	bp.SetExportFn(func(batch [][]byte) error {
		mu.Lock()
		defer mu.Unlock()
		exported = append(exported, batch...)
		exportCount.Add(1)
		return nil
	})

	bp.Add([]byte("stop-item1"))
	bp.Add([]byte("stop-item2"))

	// Stop should trigger a final flush
	bp.Stop()

	mu.Lock()
	defer mu.Unlock()

	if len(exported) != 2 {
		t.Fatalf("expected 2 exported items from Stop flush, got %d", len(exported))
	}
	if string(exported[0]) != "stop-item1" {
		t.Errorf("exported[0] = %q, want %q", string(exported[0]), "stop-item1")
	}
	if string(exported[1]) != "stop-item2" {
		t.Errorf("exported[1] = %q, want %q", string(exported[1]), "stop-item2")
	}
}
