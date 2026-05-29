package wasm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WASMModule wraps a compiled WASM module with its manifest and runtime instance.
type WASMModule struct {
	Name     string
	Manifest *Manifest
	module   wazero.CompiledModule
	instance api.Module
}

// newWASMModule creates a module wrapper. The caller is responsible for
// instantiation via the runtime before calling Execute.
func newWASMModule(name string, manifest *Manifest, compiled wazero.CompiledModule) *WASMModule {
	return &WASMModule{
		Name:     name,
		Manifest: manifest,
		module:   compiled,
	}
}

// Execute invokes the module's exported "_start" or entry-point function.
// This is a stub that will be expanded with proper function dispatch.
func (m *WASMModule) Execute(ctx context.Context, input []byte) ([]byte, error) {
	if m.instance == nil {
		return nil, fmt.Errorf("wasm module %q is not instantiated", m.Name)
	}

	// Look up the entry-point function. Convention: "run" or "_start".
	entryFunc := m.instance.ExportedFunction("run")
	if entryFunc == nil {
		entryFunc = m.instance.ExportedFunction("_start")
	}
	if entryFunc == nil {
		return nil, fmt.Errorf("wasm module %q has no exported entry point (expected 'run' or '_start')", m.Name)
	}

	// Stub: invoke the entry function with no arguments.
	// Real implementation will marshal input/output via linear memory.
	_, err := entryFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("wasm module %q execution failed: %w", m.Name, err)
	}

	return input, nil // echo input as stub output
}

// Close releases the module instance resources.
func (m *WASMModule) Close(ctx context.Context) error {
	if m.instance != nil {
		return m.instance.Close(ctx)
	}
	return nil
}
