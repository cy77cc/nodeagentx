package tracing

import (
	"context"
	"testing"
)

func TestExporterExport(t *testing.T) {
	exp := NewExporter("http://localhost:4318", "http")

	err := exp.Export(context.Background(), []byte(`{"traces":"test"}`))
	if err != nil {
		t.Fatalf("Export() returned error: %v", err)
	}
}
