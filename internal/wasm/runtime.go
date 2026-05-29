package wasm

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/rs/zerolog"
	"github.com/tetratelabs/wazero"
)

// RuntimeConfig controls the WASM runtime behaviour.
type RuntimeConfig struct {
	// CacheDir is an optional directory for compiled module caches.
	CacheDir string
	// LogLevel controls wazero's internal logging verbosity (0 = disabled).
	LogLevel int
}

// WASMRuntime manages compilation, instantiation, and lifecycle of WASM modules.
type WASMRuntime struct {
	cfg    RuntimeConfig
	logger zerolog.Logger
	runtime wazero.Runtime

	mu      sync.RWMutex
	modules map[string]*WASMModule
}

// NewRuntime creates and initialises a WASM runtime backed by wazero.
func NewRuntime(ctx context.Context, cfg RuntimeConfig, logger zerolog.Logger) (*WASMRuntime, error) {
	r := wazero.NewRuntime(ctx)

	return &WASMRuntime{
		cfg:     cfg,
		logger:  logger,
		runtime: r,
		modules: make(map[string]*WASMModule),
	}, nil
}

// LoadModule reads a WASM binary from disk, compiles it, instantiates it,
// and registers it by manifest name.
func (r *WASMRuntime) LoadModule(ctx context.Context, manifestPath string) (*WASMModule, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	binaryPath := manifest.BinaryPath
	binaryData, err := readBinary(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("read wasm binary: %w", err)
	}

	compiled, err := r.runtime.CompileModule(ctx, binaryData)
	if err != nil {
		return nil, fmt.Errorf("compile wasm module %q: %w", manifest.Name, err)
	}

	// Instantiate the module with default configuration.
	instance, err := r.runtime.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().
		WithName(manifest.Name).
		WithStdout(nil).
		WithStderr(nil))
	if err != nil {
		return nil, fmt.Errorf("instantiate wasm module %q: %w", manifest.Name, err)
	}

	mod := newWASMModule(manifest.Name, manifest, compiled)
	mod.instance = instance

	r.mu.Lock()
	r.modules[manifest.Name] = mod
	r.mu.Unlock()

	r.logger.Info().
		Str("module", manifest.Name).
		Str("version", manifest.Version).
		Msg("wasm module loaded")

	return mod, nil
}

// ListModules returns the names of all currently loaded modules.
func (r *WASMRuntime) ListModules() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	return names
}

// Close shuts down the runtime and releases all resources.
func (r *WASMRuntime) Close(ctx context.Context) error {
	r.mu.Lock()
	r.modules = make(map[string]*WASMModule)
	r.mu.Unlock()

	if err := r.runtime.Close(ctx); err != nil {
		return fmt.Errorf("close wasm runtime: %w", err)
	}
	return nil
}

// readBinary reads a WASM binary file from disk.
func readBinary(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return data, nil
}
