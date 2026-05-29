package tracing

import "context"

// Exporter sends trace data to an external endpoint.
type Exporter struct {
	endpoint string
	protocol string
}

// NewExporter creates an Exporter targeting the given endpoint using the specified protocol.
func NewExporter(endpoint, protocol string) *Exporter {
	return &Exporter{
		endpoint: endpoint,
		protocol: protocol,
	}
}

// Export sends the provided data to the configured endpoint. This is a stub implementation.
func (e *Exporter) Export(_ context.Context, _ []byte) error {
	return nil
}
