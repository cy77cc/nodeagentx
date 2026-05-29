package wasm

import (
	"testing"
)

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name: "valid manifest with defaults",
			input: `name: hello
version: 1.0.0
binary_path: hello.wasm
task_types:
  - greet
`,
			wantErr: false,
		},
		{
			name: "valid manifest with explicit limits",
			input: `name: hello
version: 1.0.0
binary_path: hello.wasm
task_types:
  - greet
limits:
  max_memory_pages: 512
  max_table_size: 2048
  max_cpu_seconds: 60
`,
			wantErr: false,
		},
		{
			name: "valid manifest with sandbox config",
			input: `name: sandboxed
version: 0.1.0
binary_path: sandboxed.wasm
task_types:
  - scan
sandbox:
  enabled: true
  network_access: false
  allowed_paths:
    - /tmp
`,
			wantErr: false,
		},
		{
			name: "missing name",
			input: `version: 1.0.0
binary_path: hello.wasm
task_types:
  - greet
`,
			wantErr: true,
		},
		{
			name: "missing version",
			input: `name: hello
binary_path: hello.wasm
task_types:
  - greet
`,
			wantErr: true,
		},
		{
			name: "missing binary_path",
			input: `name: hello
version: 1.0.0
task_types:
  - greet
`,
			wantErr: true,
		},
		{
			name: "missing task_types",
			input: `name: hello
version: 1.0.0
binary_path: hello.wasm
`,
			wantErr: true,
		},
		{
			name:    "invalid yaml",
			input:   `::: not yaml`,
			wantErr: true,
		},
		{
			name: "defaults applied for runtime and limits",
			input: `name: defmod
version: 0.0.1
binary_path: defmod.wasm
task_types:
  - run
`,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := ParseManifest([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if m.Name == "" {
				t.Error("name should not be empty")
			}
			if m.Runtime == "" {
				t.Error("runtime should have been defaulted")
			}
			if m.Limits.MaxMemoryPages == 0 {
				t.Error("max_memory_pages should have been defaulted")
			}
			if m.Limits.MaxTableSize == 0 {
				t.Error("max_table_size should have been defaulted")
			}
			if m.Limits.MaxCPUSeconds == 0 {
				t.Error("max_cpu_seconds should have been defaulted")
			}
		})
	}
}
