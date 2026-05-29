package tracing

import (
	"sync"
	"time"
)

// BatchConfig configures the batch processor behavior.
type BatchConfig struct {
	MaxSize int
	Timeout time.Duration
}

// BatchProcessor accumulates items and flushes them in batches.
type BatchProcessor struct {
	cfg      BatchConfig
	batch    [][]byte
	mu       sync.Mutex
	exportFn func([][]byte) error
	done     chan struct{}
}

// NewBatchProcessor creates a BatchProcessor and starts its flush loop goroutine.
func NewBatchProcessor(cfg BatchConfig) *BatchProcessor {
	bp := &BatchProcessor{
		cfg:   cfg,
		batch: make([][]byte, 0, cfg.MaxSize),
		done:  make(chan struct{}),
	}
	go bp.flushLoop()
	return bp
}

// SetExportFn sets the callback invoked on each flush.
func (bp *BatchProcessor) SetExportFn(fn func([][]byte) error) {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.exportFn = fn
}

// Add appends data to the current batch. If the batch reaches MaxSize, it is flushed.
func (bp *BatchProcessor) Add(data []byte) {
	bp.mu.Lock()
	bp.batch = append(bp.batch, data)
	shouldFlush := len(bp.batch) >= bp.cfg.MaxSize
	bp.mu.Unlock()

	if shouldFlush {
		bp.flush()
	}
}

// flush extracts the current batch and calls exportFn.
func (bp *BatchProcessor) flush() {
	bp.mu.Lock()
	if len(bp.batch) == 0 {
		bp.mu.Unlock()
		return
	}
	batch := bp.batch
	bp.batch = make([][]byte, 0, bp.cfg.MaxSize)
	exportFn := bp.exportFn
	bp.mu.Unlock()

	if exportFn != nil {
		exportFn(batch)
	}
}

// flushLoop periodically flushes the batch based on the configured timeout.
func (bp *BatchProcessor) flushLoop() {
	ticker := time.NewTicker(bp.cfg.Timeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bp.flush()
		case <-bp.done:
			return
		}
	}
}

// Stop stops the flush loop and performs a final flush of any remaining items.
func (bp *BatchProcessor) Stop() {
	close(bp.done)
	bp.flush()
}
