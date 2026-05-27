// Package otlp implements an OTLP output plugin for exporting metrics via
// the OpenTelemetry Protocol. This is a skeleton implementation; actual OTLP
// export will be added when the OTLP SDK dependency is introduced.
package otlp

import (
	"context"
	"fmt"

	"github.com/cy77cc/opsagent/internal/collector"
)

const (
	defaultProtocol = "grpc"
	defaultBatchSize = 512
	defaultTimeout   = 10
)

func init() {
	collector.RegisterOutput("otlp", func() collector.Output {
		return &OTLPOutput{}
	})
}

// OTLPOutput is a skeleton output plugin that will export metrics via OTLP.
type OTLPOutput struct {
	Endpoint    string
	Protocol    string
	Headers     map[string]string
	Compression string
	BatchSize   int
	Timeout     int
}

// Init configures the OTLP output from the provided config map.
// Endpoint is required; all other fields have sensible defaults.
func (o *OTLPOutput) Init(cfg map[string]interface{}) error {
	endpoint, ok := cfg["endpoint"].(string)
	if !ok || endpoint == "" {
		return fmt.Errorf("otlp output: endpoint is required")
	}
	o.Endpoint = endpoint

	// Apply defaults.
	o.Protocol = defaultProtocol
	o.BatchSize = defaultBatchSize
	o.Timeout = defaultTimeout

	// Override with provided config values.
	if v, ok := cfg["protocol"].(string); ok && v != "" {
		o.Protocol = v
	}
	if v, ok := cfg["headers"].(map[string]string); ok {
		o.Headers = v
	}
	if v, ok := cfg["compression"].(string); ok {
		o.Compression = v
	}
	if v, ok := cfg["batch_size"].(int); ok && v > 0 {
		o.BatchSize = v
	}
	if v, ok := cfg["timeout"].(int); ok && v > 0 {
		o.Timeout = v
	}

	return nil
}

// Write is a skeleton stub. It will be replaced with actual OTLP export logic
// when the OTLP SDK dependency is added.
func (o *OTLPOutput) Write(ctx context.Context, metrics []collector.Metric) error {
	if len(metrics) == 0 {
		return nil
	}
	// TODO: implement actual OTLP metric export
	return nil
}

// Close is a no-op for the OTLP output.
func (o *OTLPOutput) Close() error {
	return nil
}

// SampleConfig returns a sample configuration for the OTLP output.
func (o *OTLPOutput) SampleConfig() string {
	return `
  [outputs.otlp]
    endpoint = "localhost:4317"
    protocol = "grpc"
    headers = {"Authorization" = "Bearer token"}
    compression = "gzip"
    batch_size = 512
    timeout = 10
`
}
