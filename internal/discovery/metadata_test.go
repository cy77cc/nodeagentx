package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataLayerName(t *testing.T) {
	layer := NewMetadataLayer()
	if got := layer.Name(); got != "metadata" {
		t.Errorf("Name() = %q, want %q", got, "metadata")
	}
}

func TestMetadataLayerWithMock(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/instance-id", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "i-0abc123def456")
	})
	mux.HandleFunc("/instance-type", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "t3.micro")
	})
	mux.HandleFunc("/placement/region", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "us-east-1")
	})
	mux.HandleFunc("/local-ipv4", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "10.0.0.42")
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	layer := &MetadataLayer{
		metadataURL: srv.URL + "/",
		client:      srv.Client(),
	}

	services, err := layer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("Discover() returned %d services, want 1", len(services))
	}

	svc := services[0]

	if svc.Type != "cloud_metadata" {
		t.Errorf("Type = %q, want %q", svc.Type, "cloud_metadata")
	}
	if svc.Name != "i-0abc123def456" {
		t.Errorf("Name = %q, want %q", svc.Name, "i-0abc123def456")
	}
	if svc.Labels["cloud"] != "aws" {
		t.Errorf("Labels[cloud] = %q, want %q", svc.Labels["cloud"], "aws")
	}
	if svc.Labels["region"] != "us-east-1" {
		t.Errorf("Labels[region] = %q, want %q", svc.Labels["region"], "us-east-1")
	}
	if svc.Metadata["instance_id"] != "i-0abc123def456" {
		t.Errorf("Metadata[instance_id] = %v, want %q", svc.Metadata["instance_id"], "i-0abc123def456")
	}
	if svc.Metadata["instance_type"] != "t3.micro" {
		t.Errorf("Metadata[instance_type] = %v, want %q", svc.Metadata["instance_type"], "t3.micro")
	}
	if svc.Metadata["local_ip"] != "10.0.0.42" {
		t.Errorf("Metadata[local_ip] = %v, want %q", svc.Metadata["local_ip"], "10.0.0.42")
	}
}

func TestMetadataLayerNoInstanceID(t *testing.T) {
	mux := http.NewServeMux()
	// Return 404 for instance-id to simulate no metadata available.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	layer := &MetadataLayer{
		metadataURL: srv.URL + "/",
		client:      srv.Client(),
	}

	services, err := layer.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() returned error: %v", err)
	}
	if services != nil {
		t.Errorf("Discover() returned %d services, want nil when instance_id is empty", len(services))
	}
}
