package wasm

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest describes a WASM module's metadata, capabilities, and resource limits.
type Manifest struct {
	Name         string         `yaml:"name"`
	Version      string         `yaml:"version"`
	Runtime      string         `yaml:"runtime"`
	BinaryPath   string         `yaml:"binary_path"`
	TaskTypes    []string       `yaml:"task_types"`
	Limits       Limits         `yaml:"limits"`
	SandboxConfig SandboxConfig `yaml:"sandbox"`
}

// Limits specifies resource constraints for a WASM module.
type Limits struct {
	MaxMemoryPages int `yaml:"max_memory_pages"`
	MaxTableSize   int `yaml:"max_table_size"`
	MaxCPUSeconds  int `yaml:"max_cpu_seconds"`
}

// SandboxConfig controls sandboxing behaviour for the WASM module.
type SandboxConfig struct {
	Enabled       bool     `yaml:"enabled"`
	NetworkAccess bool     `yaml:"network_access"`
	AllowedPaths  []string `yaml:"allowed_paths"`
}

// defaultLimits returns sensible defaults for module resource limits.
func defaultLimits() Limits {
	return Limits{
		MaxMemoryPages: 256, // 256 * 64 KiB = 16 MiB
		MaxTableSize:   1024,
		MaxCPUSeconds:  30,
	}
}

// defaultSandboxConfig returns the default sandbox configuration.
func defaultSandboxConfig() SandboxConfig {
	return SandboxConfig{
		Enabled:       true,
		NetworkAccess: false,
		AllowedPaths:  nil,
	}
}

// ParseManifest parses YAML bytes into a Manifest and applies defaults.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse wasm manifest: %w", err)
	}

	if err := m.validate(); err != nil {
		return nil, err
	}

	// Apply defaults for zero-valued fields.
	if m.Runtime == "" {
		m.Runtime = "wasm"
	}
	if m.Limits.MaxMemoryPages == 0 {
		m.Limits.MaxMemoryPages = defaultLimits().MaxMemoryPages
	}
	if m.Limits.MaxTableSize == 0 {
		m.Limits.MaxTableSize = defaultLimits().MaxTableSize
	}
	if m.Limits.MaxCPUSeconds == 0 {
		m.Limits.MaxCPUSeconds = defaultLimits().MaxCPUSeconds
	}

	return &m, nil
}

// LoadManifest reads a manifest file from disk and parses it.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read wasm manifest %s: %w", path, err)
	}
	return ParseManifest(data)
}

// validate checks that required fields are present.
func (m *Manifest) validate() error {
	if m.Name == "" {
		return fmt.Errorf("wasm manifest: name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("wasm manifest: version is required")
	}
	if m.BinaryPath == "" {
		return fmt.Errorf("wasm manifest: binary_path is required")
	}
	if len(m.TaskTypes) == 0 {
		return fmt.Errorf("wasm manifest: task_types must not be empty")
	}
	return nil
}
