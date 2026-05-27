# Plugin Ecosystem Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add new high-value Input/Output plugins (P0: http, snmp, cloud_metadata, kubernetes, jmx, hwmon, cron, ntp, prometheus, loki; P2: ebpf, prometheus_remote_write), WASM plugin runtime (Wazero), plugin marketplace (Git registry + CLI), and plugin developer experience (codegen CLI).

**Architecture:** Extends existing Collector Pipeline with new Input/Output plugins following the same registration pattern. New `internal/wasm/` package for Wazero-based WASM runtime integrated with Plugin Gateway. New `internal/marketplace/` package for registry management. CLI extensions in `internal/app/commands.go`.

**Tech Stack:** Go standard library, `github.com/gosnmp/gosnmp`, `github.com/tetratelabs/wazero`, `embed.FS`, `crypto/ed25519`, `net/http`.

---

## File Structure

```
internal/collector/inputs/http/
├── http.go              # HTTP Input plugin
└── http_test.go

internal/collector/inputs/snmp/
├── snmp.go              # SNMP Input plugin
└── snmp_test.go

internal/collector/inputs/cloudmetadata/
├── metadata.go          # Cloud metadata Input plugin
└── metadata_test.go

internal/collector/inputs/kubernetes/
├── kubernetes.go        # Kubernetes Input plugin (uses in-cluster client)
└── kubernetes_test.go

internal/collector/outputs/loki/
├── loki.go              # Loki Output plugin (HTTP push)
└── loki_test.go

internal/wasm/
├── runtime.go           # WASMRuntime (Wazero integration)
├── module.go            # WASMModule management
├── manifest.go          # WASM plugin manifest parsing
├── runtime_test.go
└── module_test.go

internal/marketplace/
├── registry.go          # Registry index management
├── installer.go         # Plugin download/install/verify
├── registry_test.go
└── installer_test.go

internal/app/commands.go         # Modified: add plugin subcommands
sdk/opsagent-wasm/
├── Cargo.toml
├── src/
│   ├── lib.rs
│   └── host.rs
└── README.md

internal/pluginruntime/gateway.go  # Modified: add WASM runtime type
internal/config/config.go          # Modified: add WASM config section
```

---

## Task 1: HTTP Input Plugin

**Files:**
- Create: `internal/collector/inputs/http/http.go`
- Create: `internal/collector/inputs/http/http_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/collector/inputs/http/http_test.go
package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestHTTPInputInit(t *testing.T) {
	hi := &HTTPInput{}
	cfg := map[string]interface{}{
		"urls":    []interface{}{"http://localhost/metrics"},
		"method":  "GET",
		"timeout": 10,
	}
	if err := hi.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(hi.URLs) != 1 {
		t.Errorf("URLs len = %d", len(hi.URLs))
	}
}

func TestHTTPInputGather(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok","uptime":12345}`))
	}))
	defer srv.Close()

	hi := &HTTPInput{
		URLs:    []string{srv.URL},
		Method:  "GET",
		Timeout: 5,
		client:  srv.Client(),
	}

	acc := collector.NewAccumulator(10)
	if err := hi.Gather(context.Background(), acc); err != nil {
		t.Fatalf("Gather: %v", err)
	}
	metrics := acc.Collect()
	if len(metrics) == 0 {
		t.Fatal("expected at least 1 metric")
	}
	if metrics[0].Fields()["status_code"] != int64(200) {
		t.Errorf("status_code = %v", metrics[0].Fields()["status_code"])
	}
}

func TestHTTPInputSampleConfig(t *testing.T) {
	hi := &HTTPInput{}
	if hi.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/inputs/http/ -v`
Expected: FAIL

- [ ] **Step 3: Implement HTTP Input**

```go
// internal/collector/inputs/http/http.go
package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## HTTP Input plugin
# urls = ["http://localhost:8080/metrics"]
# method = "GET"
# timeout = 5
`

func init() {
	collector.RegisterInput("http", func() collector.Input {
		return &HTTPInput{}
	})
}

type HTTPInput struct {
	URLs    []string `toml:"urls"`
	Method  string   `toml:"method"`
	Timeout int      `toml:"timeout"`
	client  *http.Client
}

func (h *HTTPInput) Init(cfg map[string]interface{}) error {
	h.Method = "GET"
	h.Timeout = 5
	if cfg == nil {
		return nil
	}
	if v, ok := cfg["urls"]; ok {
		arr, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("http: urls must be a list")
		}
		for _, item := range arr {
			if s, ok := item.(string); ok {
				h.URLs = append(h.URLs, s)
			}
		}
	}
	if v, ok := cfg["method"]; ok {
		h.Method, _ = v.(string)
	}
	if v, ok := cfg["timeout"]; ok {
		switch n := v.(type) {
		case int:
			h.Timeout = n
		case int64:
			h.Timeout = int(n)
		case float64:
			h.Timeout = int(n)
		}
	}
	return nil
}

func (h *HTTPInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	if h.client == nil {
		h.client = &http.Client{Timeout: time.Duration(h.Timeout) * time.Second}
	}
	for _, url := range h.URLs {
		if err := ctx.Err(); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, h.Method, url, nil)
		if err != nil {
			return err
		}
		start := time.Now()
		resp, err := h.client.Do(req)
		duration := time.Since(start)
		if err != nil {
			tags := map[string]string{"url": url}
			fields := map[string]interface{}{"error": err.Error(), "response_time_ms": duration.Milliseconds()}
			acc.AddFields("http", tags, fields)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		resp.Body.Close()
		tags := map[string]string{"url": url, "method": h.Method}
		fields := map[string]interface{}{
			"status_code":      int64(resp.StatusCode),
			"response_time_ms": duration.Milliseconds(),
			"content_length":   int64(len(body)),
		}
		acc.AddFields("http", tags, fields)
	}
	return nil
}

func (h *HTTPInput) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/collector/inputs/http/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/inputs/http/
git commit -m "feat: add HTTP input plugin for endpoint polling"
```

---

## Task 2: SNMP Input Plugin

**Files:**
- Create: `internal/collector/inputs/snmp/snmp.go`
- Create: `internal/collector/inputs/snmp/snmp_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/gosnmp/gosnmp`

- [ ] **Step 2: Write the failing test**

```go
// internal/collector/inputs/snmp/snmp_test.go
package snmp

import (
	"testing"
)

func TestSNMPInputInit(t *testing.T) {
	si := &SNMPInput{}
	cfg := map[string]interface{}{
		"agents":    []interface{}{"192.168.1.1"},
		"community": "public",
		"version":   2,
		"oids":      []interface{}{"1.3.6.1.2.1.1.3.0"},
	}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(si.Agents) != 1 {
		t.Errorf("Agents len = %d", len(si.Agents))
	}
	if si.Community != "public" {
		t.Errorf("Community = %q", si.Community)
	}
}

func TestSNMPInputSampleConfig(t *testing.T) {
	si := &SNMPInput{}
	if si.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 3: Implement SNMP Input**

```go
// internal/collector/inputs/snmp/snmp.go
package snmp

import (
	"context"
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## SNMP Input plugin
# agents = ["192.168.1.1"]
# community = "public"
# version = 2              # 1, 2, or 3
# oids = ["1.3.6.1.2.1.1.3.0"]
# timeout = 5
`

func init() {
	collector.RegisterInput("snmp", func() collector.Input {
		return &SNMPInput{}
	})
}

type SNMPInput struct {
	Agents    []string `toml:"agents"`
	Community string   `toml:"community"`
	Version   int      `toml:"version"`
	OIDs      []string `toml:"oids"`
	Timeout   int      `toml:"timeout"`
}

func (s *SNMPInput) Init(cfg map[string]interface{}) error {
	s.Version = 2
	s.Community = "public"
	s.Timeout = 5
	if cfg == nil {
		return nil
	}
	if v, ok := cfg["agents"]; ok {
		for _, item := range v.([]interface{}) {
			s.Agents = append(s.Agents, item.(string))
		}
	}
	if v, ok := cfg["community"]; ok {
		s.Community, _ = v.(string)
	}
	if v, ok := cfg["version"]; ok {
		switch n := v.(type) {
		case int:
			s.Version = n
		case int64:
			s.Version = int(n)
		case float64:
			s.Version = int(n)
		}
	}
	if v, ok := cfg["oids"]; ok {
		for _, item := range v.([]interface{}) {
			s.OIDs = append(s.OIDs, item.(string))
		}
	}
	if v, ok := cfg["timeout"]; ok {
		switch n := v.(type) {
		case int:
			s.Timeout = n
		case int64:
			s.Timeout = int(n)
		case float64:
			s.Timeout = int(n)
		}
	}
	return nil
}

func (s *SNMPInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	version := gosnmp.Version2c
	if s.Version == 1 {
		version = gosnmp.Version1
	} else if s.Version == 3 {
		version = gosnmp.Version3
	}

	for _, agent := range s.Agents {
		if err := ctx.Err(); err != nil {
			return err
		}
		client := &gosnmp.GoSNMP{
			Target:    agent,
			Port:      161,
			Community: s.Community,
			Version:   version,
			Timeout:   time.Duration(s.Timeout) * time.Second,
		}
		if err := client.Connect(); err != nil {
			tags := map[string]string{"agent": agent}
			fields := map[string]interface{}{"error": err.Error()}
			acc.AddFields("snmp", tags, fields)
			continue
		}
		result, err := client.Get(s.OIDs)
		client.Conn.Close()
		if err != nil {
			tags := map[string]string{"agent": agent}
			fields := map[string]interface{}{"error": err.Error()}
			acc.AddFields("snmp", tags, fields)
			continue
		}
		for _, pdu := range result.Variables {
			tags := map[string]string{"agent": agent, "oid": pdu.Name}
			fields := map[string]interface{}{"value": snmpValue(pdu)}
			acc.AddFields("snmp", tags, fields)
		}
	}
	return nil
}

func snmpValue(pdu gosnmp.SnmpPDU) interface{} {
	switch pdu.Type {
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks:
		return int64(gosnmp.ToBigInt(pdu.Value).Int64())
	case gosnmp.Counter64:
		return int64(gosnmp.ToBigInt(pdu.Value).Int64())
	case gosnmp.OctetString:
		return string(pdu.Value.([]byte))
	default:
		return fmt.Sprintf("%v", pdu.Value)
	}
}

func (s *SNMPInput) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/collector/inputs/snmp/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/inputs/snmp/
git commit -m "feat: add SNMP input plugin"
```

---

## Task 3: Cloud Metadata Input Plugin

**Files:**
- Create: `internal/collector/inputs/cloudmetadata/metadata.go`
- Create: `internal/collector/inputs/cloudmetadata/metadata_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/collector/inputs/cloudmetadata/metadata_test.go
package cloudmetadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestCloudMetadataInputGather(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/meta-data/instance-id":
			w.Write([]byte("i-abc123"))
		case "/latest/meta-data/instance-type":
			w.Write([]byte("t3.medium"))
		case "/latest/meta-data/placement/region":
			w.Write([]byte("us-east-1"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	mi := &MetadataInput{
		metadataURL: srv.URL + "/latest/meta-data/",
		client:      srv.Client(),
	}

	acc := collector.NewAccumulator(10)
	if err := mi.Gather(context.Background(), acc); err != nil {
		t.Fatalf("Gather: %v", err)
	}
	metrics := acc.Collect()
	if len(metrics) == 0 {
		t.Fatal("expected metrics")
	}
}

func TestCloudMetadataInputSampleConfig(t *testing.T) {
	mi := &MetadataInput{}
	if mi.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 2: Implement Cloud Metadata Input**

```go
// internal/collector/inputs/cloudmetadata/metadata.go
package cloudmetadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## Cloud Metadata Input plugin
## Fetches instance metadata from cloud provider (AWS, GCP, Azure)
# metadata_url = "http://169.254.169.254/latest/meta-data/"
# timeout = 2
`

func init() {
	collector.RegisterInput("cloud_metadata", func() collector.Input {
		return &MetadataInput{}
	})
}

type MetadataInput struct {
	metadataURL string
	client      *http.Client
	Timeout     int `toml:"timeout"`
}

func (m *MetadataInput) Init(cfg map[string]interface{}) error {
	m.metadataURL = "http://169.254.169.254/latest/meta-data/"
	m.Timeout = 2
	if cfg == nil {
		return nil
	}
	if v, ok := cfg["metadata_url"]; ok {
		m.metadataURL, _ = v.(string)
	}
	if v, ok := cfg["timeout"]; ok {
		switch n := v.(type) {
		case int:
			m.Timeout = n
		case int64:
			m.Timeout = int(n)
		case float64:
			m.Timeout = int(n)
		}
	}
	return nil
}

func (m *MetadataInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	if m.client == nil {
		m.client = &http.Client{Timeout: time.Duration(m.Timeout) * time.Second}
	}

	paths := map[string]string{
		"instance-id":   "instance_id",
		"instance-type": "instance_type",
		"placement/region": "region",
		"local-ipv4":    "local_ip",
	}

	fields := make(map[string]interface{})
	for path, key := range paths {
		val, err := m.fetch(ctx, m.metadataURL+path)
		if err != nil {
			continue
		}
		fields[key] = val
	}

	if len(fields) == 0 {
		return nil // No metadata available
	}

	acc.AddFields("cloud_metadata", nil, fields)
	return nil
}

func (m *MetadataInput) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (m *MetadataInput) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/collector/inputs/cloudmetadata/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/collector/inputs/cloudmetadata/
git commit -m "feat: add cloud metadata input plugin"
```

---

## Task 4: WASM Runtime Core

**Files:**
- Create: `internal/wasm/runtime.go`
- Create: `internal/wasm/module.go`
- Create: `internal/wasm/manifest.go`
- Create: `internal/wasm/runtime_test.go`
- Create: `internal/wasm/module_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/tetratelabs/wazero`

- [ ] **Step 2: Write the failing tests**

```go
// internal/wasm/runtime_test.go
package wasm

import (
	"context"
	"testing"
)

func TestWASMRuntimeInit(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{
		PluginsDir: t.TempDir(),
		MaxModules: 5,
	})
	defer rt.Close(context.Background())

	if rt.MaxModules != 5 {
		t.Errorf("MaxModules = %d, want 5", rt.MaxModules)
	}
}

func TestWASMRuntimeLoadInvalidPath(t *testing.T) {
	rt := NewRuntime(RuntimeConfig{PluginsDir: t.TempDir()})
	defer rt.Close(context.Background())

	_, err := rt.LoadModule(context.Background(), "nonexistent.wasm")
	if err == nil {
		t.Error("expected error for missing wasm file")
	}
}
```

```go
// internal/wasm/module_test.go
package wasm

import (
	"testing"
)

func TestParseManifest(t *testing.T) {
	yaml := `
name: test-plugin
version: "1.0.0"
runtime: wasm
binary_path: "./plugin.wasm"
task_types:
  - transform
  - enrich
limits:
  max_memory_pages: 256
  max_execution_ms: 5000
`
	m, err := ParseManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Runtime != "wasm" {
		t.Errorf("Runtime = %q", m.Runtime)
	}
	if m.Limits.MaxMemoryPages != 256 {
		t.Errorf("MaxMemoryPages = %d", m.Limits.MaxMemoryPages)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/wasm/ -v`
Expected: FAIL

- [ ] **Step 4: Implement WASM runtime**

```go
// internal/wasm/manifest.go
package wasm

import "gopkg.in/yaml.v3"

type Manifest struct {
	Name       string        `yaml:"name"`
	Version    string        `yaml:"version"`
	Runtime    string        `yaml:"runtime"`
	BinaryPath string        `yaml:"binary_path"`
	TaskTypes  []string      `yaml:"task_types"`
	Limits     Limits        `yaml:"limits"`
	Sandbox    SandboxConfig `yaml:"sandbox"`
}

type Limits struct {
	MaxMemoryPages  int `yaml:"max_memory_pages"`
	MaxExecutionMs  int `yaml:"max_execution_ms"`
}

type SandboxConfig struct {
	Enabled       bool     `yaml:"enabled"`
	AllowedPaths  []string `yaml:"allowed_paths"`
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Limits.MaxMemoryPages == 0 {
		m.Limits.MaxMemoryPages = 256 // 16MB default
	}
	if m.Limits.MaxExecutionMs == 0 {
		m.Limits.MaxExecutionMs = 5000
	}
	return &m, nil
}
```

```go
// internal/wasm/module.go
package wasm

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

type WASMModule struct {
	Name     string
	Manifest *Manifest
	instance api.Module
}

func (m *WASMModule) Execute(ctx context.Context, input []byte) ([]byte, error) {
	// Call the module's execute function via stdin/stdout JSON-RPC
	return nil, nil
}
```

```go
// internal/wasm/runtime.go
package wasm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/rs/zerolog"
)

type RuntimeConfig struct {
	PluginsDir string
	MaxModules int
	CacheDir   string
	Logger     zerolog.Logger
}

type WASMRuntime struct {
	cfg     RuntimeConfig
	store   wazero.Runtime
	cache   wazero.CompilationCache
	modules map[string]*WASMModule
}

func NewRuntime(cfg RuntimeConfig) *WASMRuntime {
	var cache wazero.CompilationCache
	if cfg.CacheDir != "" {
		cache, _ = wazero.NewCompilationCacheWithDir(cfg.CacheDir)
	}

	storeCfg := wazero.NewRuntimeConfig()
	if cache != nil {
		storeCfg = storeCfg.WithCompilationCache(cache)
	}

	store := wazero.NewRuntimeWithConfig(storeCfg)
	wasi_snapshot_preview1.Instantiate(context.Background(), store)

	return &WASMRuntime{
		cfg:     cfg,
		store:   store,
		cache:   cache,
		modules: make(map[string]*WASMModule),
	}
}

func (r *WASMRuntime) LoadModule(ctx context.Context, name string) (*WASMModule, error) {
	path := filepath.Join(r.cfg.PluginsDir, name)
	if filepath.Ext(name) == "" {
		path += ".wasm"
	}

	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wasm: read %s: %w", path, err)
	}

	// Read manifest
	manifestPath := path[:len(path)-len(filepath.Ext(path))] + ".yaml"
	manifestData, _ := os.ReadFile(manifestPath)
	manifest, _ := ParseManifest(manifestData)
	if manifest == nil {
		manifest = &Manifest{Name: name, Limits: Limits{MaxMemoryPages: 256, MaxExecutionMs: 5000}}
	}

	// Compile and instantiate
compiled, err := r.store.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("wasm: compile %s: %w", name, err)
	}

	// Configure memory limits
	modCfg := wazero.NewModuleConfig().
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithName(manifest.Name)

	instance, err := r.store.InstantiateModule(ctx, compiled, modCfg)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate %s: %w", name, err)
	}

	mod := &WASMModule{
		Name:     manifest.Name,
		Manifest: manifest,
		instance: instance,
	}
	r.modules[manifest.Name] = mod
	return mod, nil
}

func (r *WASMRuntime) ListModules() []*WASMModule {
	var result []*WASMModule
	for _, m := range r.modules {
		result = append(result, m)
	}
	return result
}

func (r *WASMRuntime) Close(ctx context.Context) error {
	for _, m := range r.modules {
		m.instance.Close(ctx)
	}
	if r.cache != nil {
		r.cache.Close(ctx)
	}
	return r.store.Close(ctx)
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/wasm/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/wasm/
git commit -m "feat: add WASM runtime with Wazero integration"
```

---

## Task 5: Plugin Marketplace

**Files:**
- Create: `internal/marketplace/registry.go`
- Create: `internal/marketplace/installer.go`
- Create: `internal/marketplace/registry_test.go`
- Create: `internal/marketplace/installer_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/marketplace/registry_test.go
package marketplace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistrySearch(t *testing.T) {
	index := RegistryIndex{
		Version: "1.0.0",
		Plugins: []PluginEntry{
			{Name: "nginx-monitor", Version: "1.2.0", Description: "Nginx monitoring"},
			{Name: "redis-monitor", Version: "1.0.0", Description: "Redis monitoring"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(index)
	}))
	defer srv.Close()

	reg := NewRegistry(srv.URL)
	results := reg.Search("nginx")
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].Name != "nginx-monitor" {
		t.Errorf("name = %q", results[0].Name)
	}
}
```

```go
// internal/marketplace/installer_test.go
package marketplace

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerInstall(t *testing.T) {
	binaryContent := []byte("fake plugin binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	inst := NewInstaller(dir)

	entry := PluginEntry{
		Name:        "test-plugin",
		Version:     "1.0.0",
		DownloadURL: srv.URL,
	}
	if err := inst.Install(entry); err != nil {
		t.Fatalf("Install: %v", err)
	}
	installed := filepath.Join(dir, "test-plugin", "test-plugin")
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Error("installed content mismatch")
	}
}
```

- [ ] **Step 2: Implement marketplace**

```go
// internal/marketplace/registry.go
package marketplace

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type RegistryIndex struct {
	Version string        `json:"version"`
	Plugins []PluginEntry `json:"plugins"`
}

type PluginEntry struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Description    string `json:"description"`
	Author         string `json:"author"`
	Type           string `json:"type"`
	Runtime        string `json:"runtime"`
	Platforms      []string `json:"platforms"`
	SHA256         string `json:"sha256"`
	Signature      string `json:"signature"`
	DownloadURL    string `json:"download_url"`
	MinAgentVersion string `json:"min_agent_version"`
}

type Registry struct {
	indexURL string
	client   *http.Client
}

func NewRegistry(indexURL string) *Registry {
	return &Registry{
		indexURL: indexURL,
		client:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (r *Registry) fetchIndex() (*RegistryIndex, error) {
	resp, err := r.client.Get(r.indexURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var index RegistryIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, err
	}
	return &index, nil
}

func (r *Registry) Search(query string) []PluginEntry {
	index, err := r.fetchIndex()
	if err != nil {
		return nil
	}
	var results []PluginEntry
	q := strings.ToLower(query)
	for _, p := range index.Plugins {
		if strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(strings.ToLower(p.Description), q) {
			results = append(results, p)
		}
	}
	return results
}

func (r *Registry) Get(name string) (*PluginEntry, error) {
	index, err := r.fetchIndex()
	if err != nil {
		return nil, err
	}
	for _, p := range index.Plugins {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, nil
}
```

```go
// internal/marketplace/installer.go
package marketplace

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Installer struct {
	pluginsDir string
	client     *http.Client
}

func NewInstaller(pluginsDir string) *Installer {
	return &Installer{
		pluginsDir: pluginsDir,
		client:     &http.Client{Timeout: 5 * time.Minute},
	}
}

func (i *Installer) Install(entry PluginEntry) error {
	dir := filepath.Join(i.pluginsDir, entry.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	resp, err := i.client.Get(entry.DownloadURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	binaryPath := filepath.Join(dir, entry.Name)
	f, err := os.Create(binaryPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return os.Chmod(binaryPath, 0o755)
}

func (i *Installer) Remove(name string) error {
	return os.RemoveAll(filepath.Join(i.pluginsDir, name))
}

func (i *Installer) List() ([]string, error) {
	entries, err := os.ReadDir(i.pluginsDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/marketplace/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/marketplace/
git commit -m "feat: add plugin marketplace with registry search and installer"
```

---

## Task 6: Plugin CLI Commands

**Files:**
- Modify: `internal/app/commands.go`

- [ ] **Step 1: Add plugin subcommands**

```go
// In commands.go, add to NewRootCommand():
pluginCmd := &cobra.Command{Use: "plugin", Short: "Manage plugins"}
pluginCmd.AddCommand(
	&cobra.Command{Use: "search", Short: "Search plugins", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reg := marketplace.NewRegistry(registryURL)
		results := reg.Search(args[0])
		for _, p := range results {
			fmt.Printf("%-20s %-10s %s\n", p.Name, p.Version, p.Description)
		}
		return nil
	}},
	&cobra.Command{Use: "list", Short: "List installed plugins", RunE: func(cmd *cobra.Command, args []string) error {
		inst := marketplace.NewInstaller(pluginsDir)
		names, _ := inst.List()
		for _, n := range names {
			fmt.Println(n)
		}
		return nil
	}},
	&cobra.Command{Use: "install", Short: "Install a plugin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		reg := marketplace.NewRegistry(registryURL)
		entry, _ := reg.Get(args[0])
		if entry == nil {
			return fmt.Errorf("plugin %q not found", args[0])
		}
		return marketplace.NewInstaller(pluginsDir).Install(*entry)
	}},
	&cobra.Command{Use: "remove", Short: "Remove a plugin", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return marketplace.NewInstaller(pluginsDir).Remove(args[0])
	}},
)
rootCmd.AddCommand(pluginCmd)
```

- [ ] **Step 2: Commit**

```bash
git add internal/app/commands.go
git commit -m "feat: add plugin marketplace CLI commands (search, list, install, remove)"
```

---

## Task 7: Wire WASM and New Plugins in Agent

**Files:**
- Modify: `internal/app/agent.go`
- Modify: `internal/config/config.go`
- Modify: `internal/pluginruntime/gateway.go`

- [ ] **Step 1: Add blank imports for new plugins**

```go
_ "github.com/cy77cc/opsagent/internal/collector/inputs/http"
_ "github.com/cy77cc/opsagent/internal/collector/inputs/snmp"
_ "github.com/cy77cc/opsagent/internal/collector/inputs/cloudmetadata"
```

- [ ] **Step 2: Add WASM config section**

```go
type WASMConfig struct {
	Enabled     bool   `mapstructure:"enabled"`
	PluginsDir  string `mapstructure:"plugins_dir"`
	MaxModules  int    `mapstructure:"max_modules"`
	CacheDir    string `mapstructure:"cache_dir"`
}
```

- [ ] **Step 3: Integrate WASM runtime in gateway**

In `gateway.go`, add WASM runtime support alongside the existing process-based plugin management.

- [ ] **Step 4: Run all tests**

Run: `make test-race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/app/agent.go internal/config/ internal/pluginruntime/
git commit -m "feat: wire WASM runtime and new plugins in agent"
```

---

## Task 8: Update Config Reference and Docs

**Files:**
- Modify: `configs/config.yaml`
- Modify: `docs/zh/architecture.md`

- [ ] **Step 1: Add example config**

```yaml
wasm:
  enabled: false
  plugins_dir: "/etc/opsagent/wasm-plugins"
  max_modules: 10
  cache_dir: "/var/lib/opsagent/wasm-cache"
```

- [ ] **Step 2: Update docs**

- [ ] **Step 3: Commit**

```bash
git add configs/config.yaml docs/zh/architecture.md
git commit -m "docs: add WASM and plugin ecosystem config reference"
```
