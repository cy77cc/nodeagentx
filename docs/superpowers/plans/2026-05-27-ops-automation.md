# Ops Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add service auto-discovery (systemd/proc/container/metadata layers), config templates (embed.FS + CLI), batch management labels (gRPC proto extension), and auto-update (A/B binary swap with Ed25519 verification).

**Architecture:** New `internal/discovery/` package with layered discovery strategy (systemd, /proc, container, cloud metadata). New `internal/templates/` package with embedded YAML templates and CLI commands. New `internal/updater/` package for A/B binary updates. Extends gRPC proto with discovery report, label management, and update messages.

**Tech Stack:** Go standard library, `gopsutil` (existing), `embed.FS`, `crypto/ed25519`, `os/exec` (systemctl), `net/http` (Docker socket, cloud metadata).

---

## File Structure

```
internal/discovery/
├── discovery.go         # DiscoveryService, DiscoveryLayer interface, Service type
├── systemd.go           # SystemdLayer (systemctl list-units)
├── proc.go              # ProcLayer (/proc port scanning via gopsutil)
├── container.go         # ContainerLayer (Docker socket API)
├── metadata.go          # MetadataLayer (cloud metadata 169.254.169.254)
├── discovery_test.go
├── systemd_test.go
├── proc_test.go
├── container_test.go
└── metadata_test.go

internal/templates/
├── embed.go             # embed.FS for templates/
├── loader.go            # Template loading, variable substitution
├── loader_test.go
└── templates/
    ├── nginx.yaml
    ├── postgres.yaml
    ├── redis.yaml
    ├── system.yaml
    ├── docker.yaml
    └── mysql.yaml

internal/updater/
├── updater.go           # A/B binary swap logic
├── updater_test.go

internal/config/config.go    # Modified: add Discovery, Updater config sections
internal/app/agent.go        # Modified: wire DiscoveryService
internal/app/commands.go     # Modified: add templates subcommand
proto/agent.proto            # Modified: add ServiceDiscoveryReport, AgentUpdate, labels
```

---

## Task 1: Discovery Core Types and Interface

**Files:**
- Create: `internal/discovery/discovery.go`
- Create: `internal/discovery/discovery_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/discovery/discovery_test.go
package discovery

import (
	"context"
	"testing"
	"time"
)

type mockLayer struct {
	name     string
	services []Service
	err      error
}

func (m *mockLayer) Name() string { return m.name }
func (m *mockLayer) Discover(ctx context.Context) ([]Service, error) {
	return m.services, m.err
}

func TestDiscoveryServiceRun(t *testing.T) {
	layer := &mockLayer{
		name: "mock",
		services: []Service{
			{Name: "nginx", Type: "systemd", PID: 1234, Ports: []int{80}},
		},
	}

	ds := NewDiscoveryService(Config{
		Interval: 100 * time.Millisecond,
		Layers:   []DiscoveryLayer{layer},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	results := ds.Run(ctx)
	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1", len(results))
	}
	if results[0].Name != "nginx" {
		t.Errorf("name = %q, want nginx", results[0].Name)
	}
	if results[0].PID != 1234 {
		t.Errorf("pid = %d, want 1234", results[0].PID)
	}
}

func TestDiscoveryServiceNoLayers(t *testing.T) {
	ds := NewDiscoveryService(Config{Interval: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	results := ds.Run(ctx)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discovery/ -run TestDiscoveryService -v`
Expected: FAIL

- [ ] **Step 3: Implement discovery core**

```go
// internal/discovery/discovery.go
package discovery

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

type Service struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	PID          int               `json:"pid"`
	Ports        []int             `json:"ports"`
	Labels       map[string]string `json:"labels"`
	Metadata     map[string]string `json:"metadata"`
	DiscoveredAt time.Time         `json:"discovered_at"`
}

type DiscoveryLayer interface {
	Name() string
	Discover(ctx context.Context) ([]Service, error)
}

type Config struct {
	Interval time.Duration
	Layers   []DiscoveryLayer
	Logger   zerolog.Logger
}

type DiscoveryService struct {
	cfg      Config
	mu       sync.RWMutex
	lastRun  []Service
}

func NewDiscoveryService(cfg Config) *DiscoveryService {
	return &DiscoveryService{cfg: cfg}
}

func (ds *DiscoveryService) Run(ctx context.Context) []Service {
	if len(ds.cfg.Layers) == 0 {
		return nil
	}

	// Run once immediately
	services := ds.discover(ctx)
	ds.mu.Lock()
	ds.lastRun = services
	ds.mu.Unlock()

	// Run periodically
	ticker := time.NewTicker(ds.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ds.lastRun
		case <-ticker.C:
			services = ds.discover(ctx)
			ds.mu.Lock()
			ds.lastRun = services
			ds.mu.Unlock()
		}
	}
}

func (ds *DiscoveryService) LastResults() []Service {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	result := make([]Service, len(ds.lastRun))
	copy(result, ds.lastRun)
	return result
}

func (ds *DiscoveryService) discover(ctx context.Context) []Service {
	var all []Service
	seen := make(map[string]bool)

	for _, layer := range ds.cfg.Layers {
		services, err := layer.Discover(ctx)
		if err != nil {
			ds.cfg.Logger.Warn().Err(err).Str("layer", layer.Name()).Msg("discovery failed")
			continue
		}
		for _, s := range services {
			key := s.Type + ":" + s.Name
			if !seen[key] {
				seen[key] = true
				s.DiscoveredAt = time.Now()
				all = append(all, s)
			}
		}
	}
	return all
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/discovery/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/discovery.go internal/discovery/discovery_test.go
git commit -m "feat: add discovery service core with layered discovery strategy"
```

---

## Task 2: systemd Discovery Layer

**Files:**
- Create: `internal/discovery/systemd.go`
- Create: `internal/discovery/systemd_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/discovery/systemd_test.go
package discovery

import (
	"context"
	"os/exec"
	"testing"
)

func TestSystemdLayerName(t *testing.T) {
	l := &SystemdLayer{}
	if l.Name() != "systemd" {
		t.Errorf("Name = %q, want systemd", l.Name())
	}
}

func TestSystemdLayerDiscoverNoSystemctl(t *testing.T) {
	if _, err := exec.LookPath("systemctl"); err != nil {
		t.Skip("systemctl not available")
	}
	l := &SystemdLayer{}
	services, err := l.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	// Should find at least some services on a systemd system
	t.Logf("discovered %d services", len(services))
	for _, s := range services[:min(5, len(services))] {
		t.Logf("  %s (PID %d, ports %v)", s.Name, s.PID, s.Ports)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discovery/ -run TestSystemdLayer -v`
Expected: FAIL

- [ ] **Step 3: Implement systemd layer**

```go
// internal/discovery/systemd.go
package discovery

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

type SystemdLayer struct{}

func (s *SystemdLayer) Name() string { return "systemd" }

func (s *SystemdLayer) Discover(ctx context.Context) ([]Service, error) {
	out, err := exec.CommandContext(ctx, "systemctl",
		"list-units", "--type=service", "--state=running", "--no-legend", "--no-pager",
		"--plain").Output()
	if err != nil {
		return nil, err
	}

	var services []Service
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		unitName := fields[0]
		if !strings.HasSuffix(unitName, ".service") {
			continue
		}
		name := strings.TrimSuffix(unitName, ".service")

		// Get PID
		pidOut, err := exec.CommandContext(ctx, "systemctl",
			"show", unitName, "--property=MainPID", "--value").Output()
		if err != nil {
			continue
		}
		pidStr := strings.TrimSpace(string(pidOut))
		pid, _ := strconv.Atoi(pidStr)
		if pid == 0 {
			continue
		}

		services = append(services, Service{
			Name:  name,
			Type:  "systemd",
			PID:   pid,
			Ports: getPortsForPID(pid),
			Labels: map[string]string{
				"unit": unitName,
			},
		})
	}
	return services, nil
}

func getPortsForPID(pid int) []int {
	// Use gopsutil to get ports for a specific PID
	// This is a simplified version
	return nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/discovery/ -run TestSystemdLayer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/systemd.go internal/discovery/systemd_test.go
git commit -m "feat: add systemd discovery layer"
```

---

## Task 3: /proc Discovery Layer

**Files:**
- Create: `internal/discovery/proc.go`
- Create: `internal/discovery/proc_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/discovery/proc_test.go
package discovery

import (
	"context"
	"testing"
)

func TestProcLayerName(t *testing.T) {
	l := &ProcLayer{}
	if l.Name() != "proc" {
		t.Errorf("Name = %q, want proc", l.Name())
	}
}

func TestProcLayerDiscover(t *testing.T) {
	l := &ProcLayer{}
	services, err := l.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	t.Logf("discovered %d services from /proc", len(services))
	for _, s := range services[:min(5, len(services))] {
		t.Logf("  %s (PID %d, ports %v)", s.Name, s.PID, s.Ports)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discovery/ -run TestProcLayer -v`
Expected: FAIL

- [ ] **Step 3: Implement /proc layer**

```go
// internal/discovery/proc.go
package discovery

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type ProcLayer struct{}

func (p *ProcLayer) Name() string { return "proc" }

func (p *ProcLayer) Discover(ctx context.Context) ([]Service, error) {
	conns, err := net.Connections("inet")
	if err != nil {
		return nil, fmt.Errorf("proc: get connections: %w", err)
	}

	// Group by PID, only LISTEN state
	pidPorts := make(map[int32][]int)
	for _, c := range conns {
		if c.Status == "LISTEN" && c.Pid > 0 {
			pidPorts[c.Pid] = append(pidPorts[c.Pid], int(c.Lport))
		}
	}

	var services []Service
	for pid, ports := range pidPorts {
		proc, err := process.NewProcess(pid)
		if err != nil {
			continue
		}
		name, _ := proc.Name()
		if name == "" {
			// Fallback: read /proc/<pid>/comm
			comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
			if err == nil {
				name = strings.TrimSpace(string(comm))
			}
		}
		if name == "" {
			continue
		}

		cmdline, _ := proc.Cmdline()

		services = append(services, Service{
			Name:  name,
			Type:  "process",
			PID:   int(pid),
			Ports: ports,
			Metadata: map[string]string{
				"cmdline": cmdline,
			},
		})
	}
	return services, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/discovery/ -run TestProcLayer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/proc.go internal/discovery/proc_test.go
git commit -m "feat: add /proc discovery layer for port-to-process mapping"
```

---

## Task 4: Container Discovery Layer

**Files:**
- Create: `internal/discovery/container.go`
- Create: `internal/discovery/container_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/discovery/container_test.go
package discovery

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContainerLayerName(t *testing.T) {
	l := &ContainerLayer{}
	if l.Name() != "container" {
		t.Errorf("Name = %q, want container", l.Name())
	}
}

func TestContainerLayerNoDocker(t *testing.T) {
	l := &ContainerLayer{DockerSocket: "/nonexistent/docker.sock"}
	services, err := l.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover should not error when socket missing: %v", err)
	}
	if len(services) != 0 {
		t.Errorf("expected 0 services, got %d", len(services))
	}
}

func TestContainerLayerWithMockDocker(t *testing.T) {
	// Mock Docker API
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"Id": "abc123",
				"Names": ["/nginx-proxy"],
				"Image": "nginx:latest",
				"State": "running",
				"Ports": [{"PrivatePort": 80, "PublicPort": 8080, "Type": "tcp"}],
				"Labels": {"app": "web"}
			}
		]`))
	}))
	defer srv.Close()

	// Use a Unix socket mock would be more realistic, but for testing HTTP works
	l := &ContainerLayer{DockerSocket: ""}
	l.dockerClient = &http.Client{}
	l.dockerURL = srv.URL

	services, err := l.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("services len = %d, want 1", len(services))
	}
	if services[0].Name != "nginx-proxy" {
		t.Errorf("name = %q", services[0].Name)
	}
	if services[0].Ports[0] != 8080 {
		t.Errorf("port = %d, want 8080", services[0].Ports[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discovery/ -run TestContainerLayer -v`
Expected: FAIL

- [ ] **Step 3: Implement container layer**

```go
// internal/discovery/container.go
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

type ContainerLayer struct {
	DockerSocket string
	dockerClient *http.Client
	dockerURL    string
}

func (c *ContainerLayer) Name() string { return "container" }

type dockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Ports  []dockerPort      `json:"Ports"`
	Labels map[string]string `json:"Labels"`
}

type dockerPort struct {
	PrivatePort int `json:"PrivatePort"`
	PublicPort  int `json:"PublicPort"`
	Type        string `json:"Type"`
}

func (c *ContainerLayer) Discover(ctx context.Context) ([]Service, error) {
	socket := c.DockerSocket
	if socket == "" {
		socket = "/var/run/docker.sock"
	}

	if _, err := os.Stat(socket); os.IsNotExist(err) {
		return nil, nil // Docker not available
	}

	client := c.dockerClient
	url := c.dockerURL
	if client == nil {
		client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socket)
				},
			},
		}
		url = "http://localhost"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url+"/containers/json?filters=%7B%22status%22%3A%5B%22running%22%5D%7D", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("container: docker api: %w", err)
	}
	defer resp.Body.Close()

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}

	var services []Service
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		var ports []int
		for _, p := range c.Ports {
			if p.PublicPort > 0 {
				ports = append(ports, p.PublicPort)
			}
		}
		labels := make(map[string]string)
		for k, v := range c.Labels {
			labels[k] = v
		}
		labels["image"] = c.Image
		labels["container_id"] = c.ID[:12]

		services = append(services, Service{
			Name:     name,
			Type:     "container",
			Ports:    ports,
			Labels:   labels,
			Metadata: map[string]string{"image": c.Image},
		})
	}
	return services, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/discovery/ -run TestContainerLayer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/container.go internal/discovery/container_test.go
git commit -m "feat: add container discovery layer for Docker"
```

---

## Task 5: Cloud Metadata Discovery Layer

**Files:**
- Create: `internal/discovery/metadata.go`
- Create: `internal/discovery/metadata_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/discovery/metadata_test.go
package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataLayerName(t *testing.T) {
	l := &MetadataLayer{}
	if l.Name() != "metadata" {
		t.Errorf("Name = %q, want metadata", l.Name())
	}
}

func TestMetadataLayerWithMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/meta-data/instance-id":
			w.Write([]byte("i-abc123"))
		case "/latest/meta-data/instance-type":
			w.Write([]byte("t3.medium"))
		case "/latest/meta-data/placement/region":
			w.Write([]byte("us-east-1"))
		case "/latest/meta-data/local-ipv4":
			w.Write([]byte("10.0.1.50"))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	l := &MetadataLayer{metadataURL: srv.URL + "/latest/meta-data/"}
	services, err := l.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("services len = %d, want 1", len(services))
	}
	svc := services[0]
	if svc.Name != "i-abc123" {
		t.Errorf("name = %q", svc.Name)
	}
	if svc.Metadata["instance_type"] != "t3.medium" {
		t.Errorf("instance_type = %q", svc.Metadata["instance_type"])
	}
	if svc.Metadata["region"] != "us-east-1" {
		t.Errorf("region = %q", svc.Metadata["region"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/discovery/ -run TestMetadataLayer -v`
Expected: FAIL

- [ ] **Step 3: Implement metadata layer**

```go
// internal/discovery/metadata.go
package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type MetadataLayer struct {
	metadataURL string
	client      *http.Client
}

func (m *MetadataLayer) Name() string { return "metadata" }

func (m *MetadataLayer) Discover(ctx context.Context) ([]Service, error) {
	url := m.metadataURL
	if url == "" {
		url = "http://169.254.169.254/latest/meta-data/"
	}
	client := m.client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}

	metadata := make(map[string]string)
	paths := map[string]string{
		"instance-id":   "instance_id",
		"instance-type": "instance_type",
		"placement/region": "region",
		"local-ipv4":    "local_ip",
	}

	for path, key := range paths {
		val, err := m.fetch(client, url+path, ctx)
		if err != nil {
			continue // Skip if not available
		}
		metadata[key] = val
	}

	if metadata["instance_id"] == "" {
		return nil, nil // No metadata available
	}

	return []Service{
		{
			Name:     metadata["instance_id"],
			Type:     "cloud_metadata",
			Metadata: metadata,
			Labels: map[string]string{
				"cloud":  "aws",
				"region": metadata["region"],
			},
		},
	}, nil
}

func (m *MetadataLayer) fetch(client *http.Client, url string, ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/discovery/ -run TestMetadataLayer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/discovery/metadata.go internal/discovery/metadata_test.go
git commit -m "feat: add cloud metadata discovery layer"
```

---

## Task 6: Config Templates (embed.FS + Loader)

**Files:**
- Create: `internal/templates/embed.go`
- Create: `internal/templates/loader.go`
- Create: `internal/templates/loader_test.go`
- Create: `internal/templates/templates/nginx.yaml`
- Create: `internal/templates/templates/system.yaml`

- [ ] **Step 1: Create template files**

```yaml
# internal/templates/templates/nginx.yaml
name: "nginx"
description: "Nginx web server monitoring"
version: "1.0.0"
requires:
  - service: "nginx"

variables:
  stub_status_url:
    description: "Nginx stub_status endpoint URL"
    default: "http://127.0.0.1:80/nginx_status"
    type: "string"
  log_path:
    description: "Nginx access log path"
    default: "/var/log/nginx/access.log"
    type: "string"

collector:
  inputs:
    - type: http
      config:
        urls: ["{{.stub_status_url}}"]
        method: "GET"
        name_override: "nginx_status"
    - type: tail
      config:
        files: ["{{.log_path}}"]
        from_beginning: false
```

```yaml
# internal/templates/templates/system.yaml
name: "system"
description: "Basic system monitoring (CPU, memory, disk, network)"
version: "1.0.0"
collector:
  inputs:
    - type: cpu
      config:
        percpu: false
    - type: memory
      config: {}
    - type: disk
      config: {}
    - type: net
      config: {}
    - type: load
      config: {}
```

- [ ] **Step 2: Write the failing test for loader**

```go
// internal/templates/loader_test.go
package templates

import (
	"testing"
)

func TestLoaderList(t *testing.T) {
	l := NewLoader()
	names := l.List()
	if len(names) == 0 {
		t.Fatal("expected at least 1 template")
	}
	found := false
	for _, n := range names {
		if n == "system" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'system' template")
	}
}

func TestLoaderGet(t *testing.T) {
	l := NewLoader()
	tmpl, err := l.Get("nginx")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tmpl.Name != "nginx" {
		t.Errorf("Name = %q", tmpl.Name)
	}
	if len(tmpl.Collector.Inputs) == 0 {
		t.Error("expected inputs")
	}
}

func TestLoaderGetNotFound(t *testing.T) {
	l := NewLoader()
	_, err := l.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestLoaderApply(t *testing.T) {
	l := NewLoader()
	tmpl, err := l.Get("nginx")
	if err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{
		"stub_status_url": "http://localhost:9113/nginx_status",
		"log_path":        "/var/log/nginx/access.log",
	}
	result, err := l.Apply(tmpl, vars)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(result.Inputs) == 0 {
		t.Error("expected inputs in result")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/templates/ -v`
Expected: FAIL

- [ ] **Step 4: Implement embed and loader**

```go
// internal/templates/embed.go
package templates

import "embed"

//go:embed templates/*.yaml
var TemplateFS embed.FS
```

```go
// internal/templates/loader.go
package templates

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Template struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Version     string            `yaml:"version"`
	Variables   map[string]VarDef `yaml:"variables"`
	Collector   TemplateCollector `yaml:"collector"`
}

type VarDef struct {
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
	Type        string `yaml:"type"`
}

type TemplateCollector struct {
	Inputs []TemplatePlugin `yaml:"inputs"`
}

type TemplatePlugin struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

type ApplyResult struct {
	Inputs []TemplatePlugin
}

type Loader struct {
	templates map[string]*Template
}

func NewLoader() *Loader {
	l := &Loader{templates: make(map[string]*Template)}
	l.loadAll()
	return l
}

func (l *Loader) loadAll() {
	entries, _ := TemplateFS.ReadDir("templates")
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		data, err := TemplateFS.ReadFile(filepath.Join("templates", entry.Name()))
		if err != nil {
			continue
		}
		var tmpl Template
		if err := yaml.Unmarshal(data, &tmpl); err != nil {
			continue
		}
		if tmpl.Name != "" {
			l.templates[tmpl.Name] = &tmpl
		}
	}
}

func (l *Loader) List() []string {
	var names []string
	for name := range l.templates {
		names = append(names, name)
	}
	return names
}

func (l *Loader) Get(name string) (*Template, error) {
	tmpl, ok := l.templates[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return tmpl, nil
}

func (l *Loader) Apply(tmpl *Template, vars map[string]string) (*ApplyResult, error) {
	// Apply variable substitution to each input config
	result := &ApplyResult{}
	for _, input := range tmpl.Collector.Inputs {
		resolved := TemplatePlugin{
			Type:   input.Type,
			Config: make(map[string]interface{}),
		}
		for k, v := range input.Config {
			switch val := v.(type) {
			case string:
				t, err := template.New("").Parse(val)
				if err != nil {
					resolved.Config[k] = val
					continue
				}
				var buf bytes.Buffer
				// Use defaults for unset variables
				effectiveVars := make(map[string]string)
				for vname, vdef := range tmpl.Variables {
					if override, ok := vars[vname]; ok {
						effectiveVars[vname] = override
					} else {
						effectiveVars[vname] = vdef.Default
					}
				}
				t.Execute(&buf, effectiveVars)
				resolved.Config[k] = buf.String()
			default:
				resolved.Config[k] = v
			}
		}
		result.Inputs = append(result.Inputs, resolved)
	}
	return result, nil
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/templates/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/templates/
git commit -m "feat: add config template library with embed.FS and variable substitution"
```

---

## Task 7: Auto-Updater

**Files:**
- Create: `internal/updater/updater.go`
- Create: `internal/updater/updater_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/updater/updater_test.go
package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdaterVerifyChecksum(t *testing.T) {
	data := []byte("test binary content")
	hash := sha256.Sum256(data)
	checksum := hex.EncodeToString(hash[:])

	u := &Updater{}
	if err := u.verifyChecksum(data, checksum); err != nil {
		t.Fatalf("verifyChecksum: %v", err)
	}
	if err := u.verifyChecksum(data, "badchecksum"); err == nil {
		t.Error("expected error for bad checksum")
	}
}

func TestUpdaterVerifySignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	data := []byte("test binary content")
	sig := ed25519.Sign(priv, data)

	u := &Updater{publicKey: pub}
	if err := u.verifySignature(data, sig); err != nil {
		t.Fatalf("verifySignature: %v", err)
	}
	if err := u.verifySignature(data, ed25519.Sign(priv, []byte("other"))); err == nil {
		t.Error("expected error for bad signature")
	}
}

func TestUpdaterDownload(t *testing.T) {
	binaryContent := []byte("fake binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(binaryContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	u := &Updater{downloadDir: dir}
	data, err := u.download(srv.URL)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(data) != string(binaryContent) {
		t.Errorf("downloaded content mismatch")
	}
}

func TestUpdaterSwap(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "opsagent")
	backupPath := filepath.Join(dir, "opsagent.bak")
	newPath := filepath.Join(dir, "new-binary")

	os.WriteFile(currentPath, []byte("old"), 0o755)
	os.WriteFile(newPath, []byte("new"), 0o755)

	u := &Updater{
		currentPath: currentPath,
		backupPath:  backupPath,
	}
	if err := u.swap(newPath); err != nil {
		t.Fatalf("swap: %v", err)
	}

	// Verify backup has old content
	backup, _ := os.ReadFile(backupPath)
	if string(backup) != "old" {
		t.Errorf("backup content = %q, want old", backup)
	}

	// Verify current has new content
	current, _ := os.ReadFile(currentPath)
	if string(current) != "new" {
		t.Errorf("current content = %q, want new", current)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/updater/ -v`
Expected: FAIL

- [ ] **Step 3: Implement updater**

```go
// internal/updater/updater.go
package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog"
)

type UpdateRequest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Signature   []byte `json:"signature"`
}

type Updater struct {
	currentPath string
	backupPath  string
	downloadDir string
	publicKey   ed25519.PublicKey
	client      *http.Client
	logger      zerolog.Logger
}

func New(currentPath, backupPath, downloadDir string, pub ed25519.PublicKey, logger zerolog.Logger) *Updater {
	return &Updater{
		currentPath: currentPath,
		backupPath:  backupPath,
		downloadDir: downloadDir,
		publicKey:   pub,
		client:      &http.Client{Timeout: 5 * time.Minute},
		logger:      logger,
	}
}

func (u *Updater) Apply(req UpdateRequest) error {
	// 1. Download
	u.logger.Info().Str("version", req.Version).Msg("downloading update")
	data, err := u.download(req.DownloadURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// 2. Verify checksum
	if err := u.verifyChecksum(data, req.SHA256); err != nil {
		return fmt.Errorf("checksum: %w", err)
	}

	// 3. Verify signature
	if err := u.verifySignature(data, req.Signature); err != nil {
		return fmt.Errorf("signature: %w", err)
	}

	// 4. Write to temp file
	tmpPath := u.downloadDir + "/opsagent-new"
	if err := os.WriteFile(tmpPath, data, 0o755); err != nil {
		return fmt.Errorf("write temp: %w", err)
	}

	// 5. Swap binaries
	if err := u.swap(tmpPath); err != nil {
		return fmt.Errorf("swap: %w", err)
	}

	u.logger.Info().Str("version", req.Version).Msg("update applied, restart required")
	return nil
}

func (u *Updater) Rollback() error {
	if _, err := os.Stat(u.backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup to rollback to")
	}
	return u.swap(u.backupPath)
}

func (u *Updater) download(url string) ([]byte, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download returned %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 500*1024*1024)) // 500MB max
}

func (u *Updater) verifyChecksum(data []byte, expected string) error {
	hash := sha256.Sum256(data)
	actual := hex.EncodeToString(hash[:])
	if actual != expected {
		return fmt.Errorf("checksum mismatch: got %s, want %s", actual, expected)
	}
	return nil
}

func (u *Updater) verifySignature(data, sig []byte) error {
	if !ed25519.Verify(u.publicKey, data, sig) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func (u *Updater) swap(newPath string) error {
	// Backup current
	if _, err := os.Stat(u.currentPath); err == nil {
		if err := os.Rename(u.currentPath, u.backupPath); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	}
	// Atomic rename
	return os.Rename(newPath, u.currentPath)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/updater/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/updater/
git commit -m "feat: add auto-updater with A/B binary swap and Ed25519 verification"
```

---

## Task 8: Config Sections + CLI Commands + Proto Extensions

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/app/commands.go`
- Modify: `proto/agent.proto`

- [ ] **Step 1: Add config structs**

```go
type DiscoveryConfig struct {
	Enabled       bool              `mapstructure:"enabled"`
	IntervalSec   int               `mapstructure:"interval_seconds"`
	Layers        []DiscoveryLayerConfig `mapstructure:"layers"`
}

type DiscoveryLayerConfig struct {
	Type    string `mapstructure:"type"`
	Enabled bool   `mapstructure:"enabled"`
}

type UpdaterConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	CurrentPath  string `mapstructure:"current_path"`
	BackupPath   string `mapstructure:"backup_path"`
	DownloadDir  string `mapstructure:"download_dir"`
}
```

Add to `Config`:
```go
Discovery DiscoveryConfig `mapstructure:"discovery"`
Updater   UpdaterConfig   `mapstructure:"updater"`
```

- [ ] **Step 2: Add templates CLI commands**

In `commands.go`, add:
```go
templatesCmd := &cobra.Command{Use: "templates", Short: "Manage config templates"}
templatesCmd.AddCommand(
	&cobra.Command{Use: "list", Short: "List available templates", RunE: func(cmd *cobra.Command, args []string) error {
		l := templates.NewLoader()
		for _, name := range l.List() {
			fmt.Println(name)
		}
		return nil
	}},
	&cobra.Command{Use: "show", Short: "Show template details", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		l := templates.NewLoader()
		tmpl, err := l.Get(args[0])
		if err != nil { return err }
		data, _ := yaml.Marshal(tmpl)
		fmt.Println(string(data))
		return nil
	}},
)
rootCmd.AddCommand(templatesCmd)
```

- [ ] **Step 3: Add proto messages**

```protobuf
message ServiceDiscoveryReport {
    string agent_id = 1;
    repeated ServiceInfo services = 2;
    int64 timestamp = 3;
}

message ServiceInfo {
    string name = 1;
    string type = 2;
    int32 pid = 3;
    repeated int32 ports = 4;
    map<string, string> labels = 5;
    map<string, string> metadata = 6;
}

message AgentUpdate {
    string version = 1;
    string download_url = 2;
    string sha256 = 3;
    bytes signature = 4;
    bool force_restart = 5;
}

message AgentUpdateAck {
    string agent_id = 1;
    string from_version = 2;
    string to_version = 3;
    string status = 4;
    string error = 5;
}
```

Add to `AgentMessage.oneof payload`: `ServiceDiscoveryReport`, `AgentUpdateAck`
Add to `AgentRegistration`: `map<string, string> labels`, `repeated string groups`

- [ ] **Step 4: Run proto generation**

Run: `make proto-gen`

- [ ] **Step 5: Run tests**

Run: `make test-race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/ internal/app/commands.go proto/
git commit -m "feat: add discovery/updater config, templates CLI, and proto extensions"
```

---

## Task 9: Wire Discovery and Updater in Agent

**Files:**
- Modify: `internal/app/agent.go`

- [ ] **Step 1: Add DiscoveryService to Agent struct**

```go
discoverySvc *discovery.DiscoveryService
updater      *updater.Updater
```

- [ ] **Step 2: Build in NewAgent()**

```go
if cfg.Discovery.Enabled {
    var layers []discovery.DiscoveryLayer
    for _, lcfg := range cfg.Discovery.Layers {
        if !lcfg.Enabled { continue }
        switch lcfg.Type {
        case "systemd":
            layers = append(layers, &discovery.SystemdLayer{})
        case "proc":
            layers = append(layers, &discovery.ProcLayer{})
        case "container":
            layers = append(layers, &discovery.ContainerLayer{})
        case "metadata":
            layers = append(layers, &discovery.MetadataLayer{})
        }
    }
    a.discoverySvc = discovery.NewDiscoveryService(discovery.Config{
        Interval: time.Duration(cfg.Discovery.IntervalSec) * time.Second,
        Layers:   layers,
        Logger:   log,
    })
}
```

- [ ] **Step 3: Start in startSubsystems()**

```go
if a.discoverySvc != nil {
    go a.discoverySvc.Run(ctx)
}
```

- [ ] **Step 4: Send discovery results in heartbeat**

In the heartbeat construction, add discovered services:
```go
if a.discoverySvc != nil {
    services := a.discoverySvc.LastResults()
    // Include in registration or separate report
}
```

- [ ] **Step 5: Run tests**

Run: `make test-race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/agent.go
git commit -m "feat: wire discovery service and updater in agent lifecycle"
```

---

## Task 10: Update Config Reference and Docs

**Files:**
- Modify: `configs/config.yaml`
- Modify: `docs/zh/architecture.md`

- [ ] **Step 1: Add example config sections**

```yaml
discovery:
  enabled: false
  interval_seconds: 300
  layers:
    - type: "systemd"
      enabled: true
    - type: "proc"
      enabled: true
    - type: "container"
      enabled: true
    - type: "metadata"
      enabled: false

updater:
  enabled: false
  current_path: "/usr/local/bin/opsagent"
  backup_path: "/usr/local/bin/opsagent.bak"
  download_dir: "/tmp/opsagent/update"
```

- [ ] **Step 2: Update architecture docs**

- [ ] **Step 3: Commit**

```bash
git add configs/config.yaml docs/zh/architecture.md
git commit -m "docs: add config reference for discovery, templates, and updater"
```
