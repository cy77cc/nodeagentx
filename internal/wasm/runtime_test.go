package wasm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestWASMRuntimeInit(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	rt, err := NewRuntime(ctx, RuntimeConfig{}, logger)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer func() {
		if err := rt.Close(ctx); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	modules := rt.ListModules()
	if len(modules) != 0 {
		t.Errorf("expected 0 modules, got %d", len(modules))
	}
}

func TestWASMRuntimeLoadInvalidPath(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	rt, err := NewRuntime(ctx, RuntimeConfig{}, logger)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer func() {
		_ = rt.Close(ctx)
	}()

	_, err = rt.LoadModule(ctx, "/nonexistent/manifest.yaml")
	if err == nil {
		t.Fatal("expected error loading from nonexistent path, got nil")
	}
}

func TestWASMRuntimeLoadInvalidBinary(t *testing.T) {
	ctx := context.Background()
	logger := zerolog.Nop()

	// Create a temporary manifest that points to a nonexistent binary.
	dir := t.TempDir()
	manifestContent := `name: test-module
version: 1.0.0
binary_path: nonexistent.wasm
task_types:
  - test
`
	manifestPath := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	rt, err := NewRuntime(ctx, RuntimeConfig{}, logger)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	defer func() {
		_ = rt.Close(ctx)
	}()

	_, err = rt.LoadModule(ctx, manifestPath)
	if err == nil {
		t.Fatal("expected error loading module with nonexistent binary, got nil")
	}
}
