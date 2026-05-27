package tracing

import (
	"context"
	"net"
	"sync"
)

// Receiver listens for incoming OTLP data on gRPC and HTTP addresses.
type Receiver struct {
	grpcAddr string
	httpAddr string

	mu     sync.Mutex
	grpcLn net.Listener
	httpLn net.Listener
}

// NewReceiver creates a Receiver bound to the given gRPC and HTTP addresses.
func NewReceiver(grpcAddr, httpAddr string) *Receiver {
	return &Receiver{
		grpcAddr: grpcAddr,
		httpAddr: httpAddr,
	}
}

// Start opens TCP listeners on the configured addresses and blocks until ctx is done.
func (r *Receiver) Start(ctx context.Context) error {
	grpcLn, err := net.Listen("tcp", r.grpcAddr)
	if err != nil {
		return err
	}

	httpLn, err := net.Listen("tcp", r.httpAddr)
	if err != nil {
		grpcLn.Close()
		return err
	}

	r.mu.Lock()
	r.grpcLn = grpcLn
	r.httpLn = httpLn
	r.mu.Unlock()

	// Block until context is cancelled
	<-ctx.Done()

	r.closeListeners()

	return ctx.Err()
}

// closeListeners closes both listeners.
func (r *Receiver) closeListeners() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.grpcLn != nil {
		r.grpcLn.Close()
		r.grpcLn = nil
	}
	if r.httpLn != nil {
		r.httpLn.Close()
		r.httpLn = nil
	}
}

// Stop closes both listeners.
func (r *Receiver) Stop() error {
	r.closeListeners()
	return nil
}
