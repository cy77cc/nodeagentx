package tracing

import (
	"context"
	"testing"
	"time"
)

func TestReceiverStartStop(t *testing.T) {
	r := NewReceiver("127.0.0.1:0", "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})

	go func() {
		close(started)
		if err := r.Start(ctx); err != nil {
			// context.Canceled is expected when we cancel
			if err != context.Canceled {
				t.Errorf("Start() returned unexpected error: %v", err)
			}
		}
	}()

	<-started
	// Give a moment for Start to set up listeners
	time.Sleep(50 * time.Millisecond)

	if err := r.Stop(); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}

	cancel()
}
