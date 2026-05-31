package cloudmetadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestCloudMetadataInputSampleConfig(t *testing.T) {
	input := &MetadataInput{}
	sc := input.SampleConfig()
	if sc == "" {
		t.Error("SampleConfig() should not be empty")
	}
}

func TestCloudMetadataInputGather(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instance-id", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("i-0abc123def456"))
	})
	mux.HandleFunc("/instance-type", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("t3.micro"))
	})
	mux.HandleFunc("/placement/region", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("us-east-1"))
	})
	mux.HandleFunc("/local-ipv4", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("10.0.0.42"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	input := &MetadataInput{}
	cfg := map[string]any{
		"metadata_url": server.URL + "/",
		"timeout":      5,
	}
	if err := input.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	acc := collector.NewAccumulator(100)
	if err := input.Gather(context.Background(), acc); err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) == 0 {
		t.Fatal("Gather() produced 0 metrics, want at least 1")
	}

	m := metrics[0]
	if m.Name() != "cloud_metadata" {
		t.Errorf("metric name = %q, want %q", m.Name(), "cloud_metadata")
	}

	expectedFields := map[string]any{
		"instance-id":   "i-0abc123def456",
		"instance-type": "t3.micro",
		"region":        "us-east-1",
		"local-ipv4":    "10.0.0.42",
	}

	fields := m.Fields()
	for key, want := range expectedFields {
		got, ok := fields[key]
		if !ok {
			t.Errorf("missing field %q", key)
			continue
		}
		if got != want {
			t.Errorf("field %q = %q, want %q", key, got, want)
		}
	}
}

func TestCloudMetadataInputInitDefaults(t *testing.T) {
	input := &MetadataInput{}
	if err := input.Init(map[string]any{}); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if input.metadataURL != defaultMetadataURL {
		t.Errorf("metadataURL = %q, want %q", input.metadataURL, defaultMetadataURL)
	}
	if input.Timeout != defaultTimeout {
		t.Errorf("Timeout = %d, want %d", input.Timeout, defaultTimeout)
	}
	if input.client == nil {
		t.Error("client should not be nil after Init()")
	}
}

func TestCloudMetadataInputInitWithConfig(t *testing.T) {
	input := &MetadataInput{}
	cfg := map[string]any{
		"metadata_url": "http://custom.metadata:8080/meta-data/",
		"timeout":      10,
	}
	if err := input.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if input.metadataURL != "http://custom.metadata:8080/meta-data/" {
		t.Errorf("metadataURL = %q, want %q", input.metadataURL, "http://custom.metadata:8080/meta-data/")
	}
	if input.Timeout != 10 {
		t.Errorf("Timeout = %d, want %d", input.Timeout, 10)
	}
}

func TestCloudMetadataInputGatherHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	input := &MetadataInput{}
	cfg := map[string]any{
		"metadata_url": server.URL + "/",
	}
	if err := input.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	acc := collector.NewAccumulator(100)
	err := input.Gather(context.Background(), acc)
	if err == nil {
		t.Error("Gather() expected error for 500 status, got nil")
	}
}
