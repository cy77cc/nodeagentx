package otlp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestOTLPOutputInit(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid config with all fields",
			cfg: map[string]interface{}{
				"endpoint":    "localhost:4317",
				"protocol":    "grpc",
				"headers":     map[string]string{"Authorization": "Bearer token"},
				"compression": "gzip",
				"batch_size":  256,
				"timeout":     30,
			},
			wantErr: false,
		},
		{
			name: "valid config with http protocol",
			cfg: map[string]interface{}{
				"endpoint": "http://localhost:4318",
				"protocol": "http",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			cfg: map[string]interface{}{
				"protocol": "grpc",
			},
			wantErr: true,
		},
		{
			name: "empty endpoint",
			cfg: map[string]interface{}{
				"endpoint": "",
			},
			wantErr: true,
		},
		{
			name: "endpoint wrong type",
			cfg: map[string]interface{}{
				"endpoint": 123,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &OTLPOutput{}
			err := o.Init(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Init() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOTLPOutputInitDefaults(t *testing.T) {
	o := &OTLPOutput{}
	err := o.Init(map[string]interface{}{
		"endpoint": "localhost:4317",
	})
	if err != nil {
		t.Fatalf("Init() unexpected error: %v", err)
	}

	if o.Protocol != "grpc" {
		t.Errorf("default Protocol = %q, want %q", o.Protocol, "grpc")
	}
	if o.BatchSize != 512 {
		t.Errorf("default BatchSize = %d, want %d", o.BatchSize, 512)
	}
	if o.Timeout != 10 {
		t.Errorf("default Timeout = %d, want %d", o.Timeout, 10)
	}
	if o.Endpoint != "localhost:4317" {
		t.Errorf("Endpoint = %q, want %q", o.Endpoint, "localhost:4317")
	}
}

func TestOTLPOutputInitMissingEndpoint(t *testing.T) {
	o := &OTLPOutput{}
	err := o.Init(map[string]interface{}{})
	if err == nil {
		t.Fatal("Init() should return error when endpoint is missing")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("error should mention endpoint, got: %v", err)
	}
}

func TestOTLPOutputSampleConfig(t *testing.T) {
	o := &OTLPOutput{}
	cfg := o.SampleConfig()
	if cfg == "" {
		t.Error("SampleConfig() should not be empty")
	}
	if !strings.Contains(cfg, "endpoint") {
		t.Error("SampleConfig() should mention endpoint")
	}
}

func TestOTLPOutputWriteEmpty(t *testing.T) {
	o := &OTLPOutput{}
	if err := o.Init(map[string]interface{}{"endpoint": "localhost:4317"}); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	err := o.Write(context.Background(), nil)
	if err != nil {
		t.Errorf("Write(nil) should return nil, got: %v", err)
	}

	err = o.Write(context.Background(), []collector.Metric{})
	if err != nil {
		t.Errorf("Write(empty) should return nil, got: %v", err)
	}
}

func TestOTLPOutputWriteStub(t *testing.T) {
	o := &OTLPOutput{}
	if err := o.Init(map[string]interface{}{"endpoint": "localhost:4317"}); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	now := time.Now()
	metrics := []collector.Metric{
		*collector.NewMetric("cpu.usage", map[string]string{"host": "s1"}, map[string]interface{}{"value": 50.0}, collector.Gauge, now),
	}

	// The stub Write should return nil (no-op).
	err := o.Write(context.Background(), metrics)
	if err != nil {
		t.Errorf("Write() stub should return nil, got: %v", err)
	}
}

func TestOTLPOutputClose(t *testing.T) {
	o := &OTLPOutput{}
	if err := o.Init(map[string]interface{}{"endpoint": "localhost:4317"}); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := o.Close(); err != nil {
		t.Errorf("Close() should return nil, got: %v", err)
	}
}
