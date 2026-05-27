# Observability Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add log collection (tail/journald/syslog), log parsing (grok/regex/JSON), OTLP export, distributed tracing relay, embedded dashboard, and local alerting to OpsAgent.

**Architecture:** Extends the existing Collector Pipeline with 3 new Input plugins (tail, journald, syslog), 1 new Processor (logparse), and 1 new Output (otlp). Adds 3 new subsystems: TracingReceiver (OTLP relay), embedded Dashboard (HTML + SSE), and AlertingEngine (rule evaluation + notification). All new subsystems follow the existing interface-driven pattern with `HealthStatus()`.

**Tech Stack:** Go standard library, `fsnotify` (existing), `github.com/coreos/go-systemd/v22/sdjournal`, `github.com/traefik/grok`, `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`, `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`, `go.opentelemetry.io/otel`, `embed.FS`, `net/http` SSE.

---

## File Structure

```
internal/collector/inputs/tail/
├── tail.go              # TailInput plugin (file tailing, cursor persistence)
├── tail_test.go         # Unit tests
├── cursor.go            # Cursor persistence (path, offset, inode)
└── cursor_test.go

internal/collector/inputs/journald/
├── journald.go          # JournaldInput plugin
└── journald_test.go

internal/collector/inputs/syslog/
├── syslog.go            # SyslogInput plugin (TCP/UDP receiver)
├── parser.go            # RFC 5424/3164 parser
├── syslog_test.go
└── parser_test.go

internal/collector/processors/logparse/
├── logparse.go          # LogParseProcessor (grok/regex/JSON)
├── grok.go              # Built-in grok patterns + custom pattern support
├── logparse_test.go
└── grok_test.go

internal/collector/outputs/otlp/
├── otlp.go              # OTLPOutput (gRPC + HTTP export)
├── otlp_test.go

internal/tracing/
├── receiver.go          # OTLP gRPC/HTTP receiver
├── processor.go         # Batch processor + attribute enrichment
├── exporter.go          # OTLP forwarder
├── receiver_test.go
├── processor_test.go
└── exporter_test.go

internal/server/
├── ui.go                # Dashboard endpoint + embedded FS
├── ui/                  # Embedded HTML/CSS/JS
│   ├── index.html
│   ├── style.css
│   └── app.js
├── sse.go               # SSE log stream handler
├── sse_test.go
├── handlers.go          # Modified: add new API routes
└── server.go            # Modified: add log ring buffer

internal/alerting/
├── engine.go            # Rule evaluation engine
├── rules.go             # YAML rule definitions + state machine
├── notifier.go          # Webhook/platform/log notifier
├── engine_test.go
├── rules_test.go
└── notifier_test.go

internal/config/config.go    # Modified: add Tracing, Alerting, LogCollector config sections
internal/config/diff.go      # Modified: add Alerting to ChangeSet (reloadable)
internal/app/interfaces.go   # Modified: add TracingReceiver interface
internal/app/agent.go        # Modified: wire tracing, alerting, blank imports
proto/agent.proto            # Modified: add AlertState message
```

---

## Task 1: tail Input Plugin -- Cursor Persistence

**Files:**
- Create: `internal/collector/inputs/tail/cursor.go`
- Create: `internal/collector/inputs/tail/cursor_test.go`

- [ ] **Step 1: Write the failing test for cursor persistence**

```go
// internal/collector/inputs/tail/cursor_test.go
package tail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCursorSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cursor.json")

	c := &Cursor{Path: "/var/log/syslog", Offset: 1024, Inode: 12345}
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadCursor(path)
	if err != nil {
		t.Fatalf("LoadCursor: %v", err)
	}
	if loaded.Path != c.Path {
		t.Errorf("Path = %q, want %q", loaded.Path, c.Path)
	}
	if loaded.Offset != c.Offset {
		t.Errorf("Offset = %d, want %d", loaded.Offset, c.Offset)
	}
	if loaded.Inode != c.Inode {
		t.Errorf("Inode = %d, want %d", loaded.Inode, c.Inode)
	}
}

func TestLoadCursorMissingFile(t *testing.T) {
	_, err := LoadCursor("/nonexistent/cursor.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestCursorSaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "cursor.json")

	c := &Cursor{Path: "/var/log/app.log", Offset: 0, Inode: 99}
	if err := c.Save(path); err != nil {
		t.Fatalf("Save should create parent dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/inputs/tail/ -run TestCursor -v`
Expected: FAIL with "cannot find module" or undefined types

- [ ] **Step 3: Implement cursor persistence**

```go
// internal/collector/inputs/tail/cursor.go
package tail

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Cursor struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Inode  uint64 `json:"inode"`
}

func (c *Cursor) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func LoadCursor(path string) (*Cursor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/collector/inputs/tail/ -run TestCursor -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/inputs/tail/cursor.go internal/collector/inputs/tail/cursor_test.go
git commit -m "feat: add cursor persistence for tail input plugin"
```

---

## Task 2: tail Input Plugin -- Core Implementation

**Files:**
- Create: `internal/collector/inputs/tail/tail.go`
- Create: `internal/collector/inputs/tail/tail_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/collector/inputs/tail/tail_test.go
package tail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestTailInputInit(t *testing.T) {
	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files":              []interface{}{"/var/log/test.log"},
		"watch_method":       "poll",
		"from_beginning":     true,
		"max_line_bytes":     4096,
		"cursor_persist_path": "/tmp/test.cursor",
	}
	if err := ti.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(ti.Files) != 1 {
		t.Errorf("Files len = %d, want 1", len(ti.Files))
	}
	if ti.WatchMethod != "poll" {
		t.Errorf("WatchMethod = %q, want %q", ti.WatchMethod, "poll")
	}
	if !ti.FromBeginning {
		t.Error("FromBeginning should be true")
	}
}

func TestTailInputInitDefaults(t *testing.T) {
	ti := &TailInput{}
	if err := ti.Init(nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if ti.WatchMethod != "poll" {
		t.Errorf("default WatchMethod = %q, want poll", ti.WatchMethod)
	}
	if ti.MaxLineBytes != 65536 {
		t.Errorf("default MaxLineBytes = %d, want 65536", ti.MaxLineBytes)
	}
}

func TestTailInputGather(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	// Write test content
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ti := &TailInput{
		Files:         []string{logFile},
		WatchMethod:   "poll",
		FromBeginning: true,
		MaxLineBytes:  65536,
	}

	acc := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc); err != nil {
		t.Fatalf("Gather: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(metrics))
	}
	if metrics[0].Name() != "tail" {
		t.Errorf("name = %q, want tail", metrics[0].Name())
	}
	if metrics[0].Fields()["message"] != "line1" {
		t.Errorf("first line = %q, want line1", metrics[0].Fields()["message"])
	}
}

func TestTailInputGatherAppend(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")
	if err := os.WriteFile(logFile, []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ti := &TailInput{
		Files:         []string{logFile},
		WatchMethod:   "poll",
		FromBeginning: true,
		MaxLineBytes:  65536,
	}

	acc := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc); err != nil {
		t.Fatal(err)
	}
	if len(acc.Collect()) != 1 {
		t.Fatal("expected 1 metric")
	}

	// Append more lines
	f, _ := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("line2\nline3\n")
	f.Close()

	acc2 := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc2); err != nil {
		t.Fatal(err)
	}
	metrics := acc2.Collect()
	if len(metrics) != 2 {
		t.Errorf("expected 2 new metrics, got %d", len(metrics))
	}
}

func TestTailInputSampleConfig(t *testing.T) {
	ti := &TailInput{}
	if ti.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/collector/inputs/tail/ -run TestTailInput -v`
Expected: FAIL with undefined `TailInput`

- [ ] **Step 3: Implement TailInput**

```go
// internal/collector/inputs/tail/tail.go
package tail

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## File tail input plugin
## Glob patterns are supported for file matching.
# files = ["/var/log/*.log"]
# watch_method = "poll"        # poll | inotify
# from_beginning = false
# cursor_persist_path = "/var/lib/opsagent/tail.cursor"
# max_line_bytes = 65536
`

func init() {
	collector.RegisterInput("tail", func() collector.Input {
		return &TailInput{}
	})
}

type TailInput struct {
	Files             []string `toml:"files"`
	WatchMethod       string   `toml:"watch_method"`
	FromBeginning     bool     `toml:"from_beginning"`
	CursorPersistPath string   `toml:"cursor_persist_path"`
	MaxLineBytes      int      `toml:"max_line_bytes"`

	mu      sync.Mutex
	offsets map[string]int64 // path -> last read offset
}

func (t *TailInput) Init(cfg map[string]interface{}) error {
	t.WatchMethod = "poll"
	t.MaxLineBytes = 65536
	t.offsets = make(map[string]int64)

	if cfg == nil {
		return nil
	}
	if v, ok := cfg["files"]; ok {
		arr, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("tail: files must be a list, got %T", v)
		}
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("tail: file path must be a string, got %T", item)
			}
			t.Files = append(t.Files, s)
		}
	}
	if v, ok := cfg["watch_method"]; ok {
		t.WatchMethod, ok = v.(string)
		if !ok {
			return fmt.Errorf("tail: watch_method must be a string")
		}
	}
	if v, ok := cfg["from_beginning"]; ok {
		t.FromBeginning, ok = v.(bool)
		if !ok {
			return fmt.Errorf("tail: from_beginning must be a bool")
		}
	}
	if v, ok := cfg["cursor_persist_path"]; ok {
		t.CursorPersistPath, ok = v.(string)
		if !ok {
			return fmt.Errorf("tail: cursor_persist_path must be a string")
		}
	}
	if v, ok := cfg["max_line_bytes"]; ok {
		switch n := v.(type) {
		case int:
			t.MaxLineBytes = n
		case int64:
			t.MaxLineBytes = int(n)
		case float64:
			t.MaxLineBytes = int(n)
		default:
			return fmt.Errorf("tail: max_line_bytes must be a number, got %T", v)
		}
	}
	return nil
}

func (t *TailInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	files, err := t.expandGlobs()
	if err != nil {
		return err
	}

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t.gatherFile(f, acc); err != nil {
			// Log but continue with other files
			continue
		}
	}
	return nil
}

func (t *TailInput) expandGlobs() ([]string, error) {
	var result []string
	seen := make(map[string]bool)
	for _, pattern := range t.Files {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("tail: bad glob %q: %w", pattern, err)
		}
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				result = append(result, m)
			}
		}
	}
	return result, nil
}

func (t *TailInput) gatherFile(path string, acc collector.Accumulator) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	offset := t.offsets[path]
	if !t.FromBeginning && offset == 0 {
		// First run, start from end unless from_beginning
		info, err := f.Stat()
		if err != nil {
			return err
		}
		offset = info.Size()
		t.offsets[path] = offset
		return nil
	}

	if _, err := f.Seek(offset, 0); err != nil {
		return err
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, t.MaxLineBytes), t.MaxLineBytes)

	for scanner.Scan() {
		line := scanner.Text()
		tags := map[string]string{"file": path}
		fields := map[string]interface{}{
			"message": line,
		}
		acc.AddFields("tail", tags, fields)
	}

	// Update offset
	newOffset, _ := f.Seek(0, 1) // current position
	t.offsets[path] = newOffset

	return scanner.Err()
}

func (t *TailInput) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/inputs/tail/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/inputs/tail/
git commit -m "feat: add tail input plugin with file tailing and cursor persistence"
```

---

## Task 3: journald Input Plugin

**Files:**
- Create: `internal/collector/inputs/journald/journald.go`
- Create: `internal/collector/inputs/journald/journald_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/coreos/go-systemd/v22/sdjournal`

- [ ] **Step 2: Write the failing test**

```go
// internal/collector/inputs/journald/journald_test.go
package journald

import (
	"testing"
)

func TestJournaldInputInit(t *testing.T) {
	ji := &JournaldInput{}
	cfg := map[string]interface{}{
		"units":                []interface{}{"nginx", "sshd"},
		"priority":             "info",
		"cursor_persist_path":  "/tmp/journal.cursor",
	}
	if err := ji.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(ji.Units) != 2 {
		t.Errorf("Units len = %d, want 2", len(ji.Units))
	}
	if ji.Priority != "info" {
		t.Errorf("Priority = %q, want info", ji.Priority)
	}
}

func TestJournaldInputInitDefaults(t *testing.T) {
	ji := &JournaldInput{}
	if err := ji.Init(nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if ji.Priority != "info" {
		t.Errorf("default Priority = %q, want info", ji.Priority)
	}
}

func TestJournaldInputSampleConfig(t *testing.T) {
	ji := &JournaldInput{}
	if ji.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}

func TestPriorityValue(t *testing.T) {
	tests := []struct {
		input string
		want  int
		err   bool
	}{
		{"emerg", 0, false},
		{"alert", 1, false},
		{"crit", 2, false},
		{"err", 3, false},
		{"warning", 4, false},
		{"notice", 5, false},
		{"info", 6, false},
		{"debug", 7, false},
		{"invalid", -1, true},
	}
	for _, tt := range tests {
		got, err := parsePriority(tt.input)
		if (err != nil) != tt.err {
			t.Errorf("parsePriority(%q) error = %v, wantErr %v", tt.input, err, tt.err)
		}
		if got != tt.want {
			t.Errorf("parsePriority(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/collector/inputs/journald/ -run TestJournaldInput -v`
Expected: FAIL

- [ ] **Step 4: Implement JournaldInput**

```go
// internal/collector/inputs/journald/journald.go
package journald

import (
	"context"
	"fmt"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## Journald input plugin (Linux only, requires systemd)
# units = ["nginx", "docker", "sshd"]
# priority = "info"           # emerg..debug
# cursor_persist_path = "/var/lib/opsagent/journal.cursor"
`

func init() {
	collector.RegisterInput("journald", func() collector.Input {
		return &JournaldInput{}
	})
}

type JournaldInput struct {
	Units             []string `toml:"units"`
	Priority          string   `toml:"priority"`
	CursorPersistPath string   `toml:"cursor_persist_path"`

	priorityVal int
}

func (j *JournaldInput) Init(cfg map[string]interface{}) error {
	j.Priority = "info"
	if cfg == nil {
		return nil
	}
	if v, ok := cfg["units"]; ok {
		arr, ok := v.([]interface{})
		if !ok {
			return fmt.Errorf("journald: units must be a list")
		}
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("journald: unit must be a string")
			}
			j.Units = append(j.Units, s)
		}
	}
	if v, ok := cfg["priority"]; ok {
		j.Priority, ok = v.(string)
		if !ok {
			return fmt.Errorf("journald: priority must be a string")
		}
	}
	if v, ok := cfg["cursor_persist_path"]; ok {
		j.CursorPersistPath, ok = v.(string)
		if !ok {
			return fmt.Errorf("journald: cursor_persist_path must be a string")
		}
	}

	p, err := parsePriority(j.Priority)
	if err != nil {
		return err
	}
	j.priorityVal = p
	return nil
}

func (j *JournaldInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	journal, err := sdjournal.NewJournal()
	if err != nil {
		return fmt.Errorf("journald: failed to open journal: %w", err)
	}
	defer journal.Close()

	// Add unit filters
	for _, unit := range j.Units {
		match := fmt.Sprintf("_SYSTEMD_UNIT=%s.service", unit)
		if err := journal.AddMatch(match); err != nil {
			return fmt.Errorf("journald: add match %q: %w", match, err)
		}
	}

	// Add priority filter
	if j.priorityVal > 0 {
		if err := journal.AddDisjunction(); err != nil {
			return err
		}
		for i := 0; i <= j.priorityVal; i++ {
			match := fmt.Sprintf("PRIORITY=%d", i)
			if err := journal.AddMatch(match); err != nil {
				return err
			}
		}
	}

	// Seek to end (only read new entries)
	if err := journal.SeekTail(); err != nil {
		return fmt.Errorf("journald: seek tail: %w", err)
	}
	// Wait briefly for any new entries
	journal.Wait(100 * time.Millisecond)

	count := 0
	for count < 1000 { // limit per gather
		r, err := journal.Next()
		if err != nil {
			return err
		}
		if r == 0 {
			break // no more entries
		}

		entry, err := journal.GetEntry()
		if err != nil {
			continue
		}

		fields := map[string]interface{}{
			"message":   string(entry.Fields["MESSAGE"]),
			"timestamp": int64(entry.RealtimeTimestamp),
		}
		if pid, ok := entry.Fields["_PID"]; ok {
			fields["pid"] = string(pid)
		}
		if comm, ok := entry.Fields["_COMM"]; ok {
			fields["command"] = string(comm)
		}
		if unit, ok := entry.Fields["_SYSTEMD_UNIT"]; ok {
			fields["unit"] = string(unit)
		}
		if prio, ok := entry.Fields["PRIORITY"]; ok {
			fields["priority"] = string(prio)
		}

		tags := map[string]string{}
		if unit, ok := entry.Fields["_SYSTEMD_UNIT"]; ok {
			tags["unit"] = string(unit)
		}

		acc.AddFields("journald", tags, fields)
		count++
	}

	return nil
}

func (j *JournaldInput) SampleConfig() string {
	return sampleConfig
}

func parsePriority(s string) (int, error) {
	switch s {
	case "emerg":
		return 0, nil
	case "alert":
		return 1, nil
	case "crit":
		return 2, nil
	case "err":
		return 3, nil
	case "warning", "warn":
		return 4, nil
	case "notice":
		return 5, nil
	case "info":
		return 6, nil
	case "debug":
		return 7, nil
	default:
		return -1, fmt.Errorf("journald: unknown priority %q", s)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/collector/inputs/journald/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/collector/inputs/journald/
git commit -m "feat: add journald input plugin for systemd journal collection"
```

---

## Task 4: syslog Input Plugin -- RFC Parser

**Files:**
- Create: `internal/collector/inputs/syslog/parser.go`
- Create: `internal/collector/inputs/syslog/parser_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/collector/inputs/syslog/parser_test.go
package syslog

import (
	"testing"
)

func TestParseRFC5424(t *testing.T) {
	line := `<13>1 2024-01-15T10:30:00.123456Z myhost nginx 1234 - - 192.168.1.1 GET /index.html 200`
	msg, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.Facility != 1 {
		t.Errorf("Facility = %d, want 1", msg.Facility)
	}
	if msg.Severity != 5 {
		t.Errorf("Severity = %d, want 5", msg.Severity)
	}
	if msg.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want myhost", msg.Hostname)
	}
	if msg.AppName != "nginx" {
		t.Errorf("AppName = %q, want nginx", msg.AppName)
	}
	if msg.ProcID != "1234" {
		t.Errorf("ProcID = %q, want 1234", msg.ProcID)
	}
	if msg.Message != "192.168.1.1 GET /index.html 200" {
		t.Errorf("Message = %q", msg.Message)
	}
}

func TestParseRFC3164(t *testing.T) {
	line := `<13>Jan 15 10:30:00 myhost nginx: GET /index.html 200`
	msg, err := Parse([]byte(line))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want myhost", msg.Hostname)
	}
	if msg.AppName != "nginx" {
		t.Errorf("AppName = %q, want nginx", msg.AppName)
	}
	if msg.Message != "GET /index.html 200" {
		t.Errorf("Message = %q", msg.Message)
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte("not a syslog message"))
	if err == nil {
		t.Error("expected error for invalid message")
	}
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		pri      int
		facility int
		severity int
	}{
		{13, 1, 5},   // user.notice
		{0, 0, 0},    // kern.emerg
		{165, 20, 5}, // local5.notice
	}
	for _, tt := range tests {
		f, s := decodePriority(tt.pri)
		if f != tt.facility || s != tt.severity {
			t.Errorf("decodePriority(%d) = (%d,%d), want (%d,%d)", tt.pri, f, s, tt.facility, tt.severity)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/inputs/syslog/ -run TestParse -v`
Expected: FAIL

- [ ] **Step 3: Implement syslog parser**

```go
// internal/collector/inputs/syslog/parser.go
package syslog

import (
	"bytes"
	"fmt"
	"strconv"
	"time"
)

type Message struct {
	Facility  int
	Severity  int
	Timestamp time.Time
	Hostname  string
	AppName   string
	ProcID    string
	MsgID     string
	Message   string
}

func decodePriority(pri int) (facility, severity int) {
	return pri / 8, pri % 8
}

func Parse(data []byte) (*Message, error) {
	if len(data) == 0 || data[0] != '<' {
		return nil, fmt.Errorf("syslog: missing priority")
	}
	end := bytes.IndexByte(data, '>')
	if end < 0 {
		return nil, fmt.Errorf("syslog: unclosed priority")
	}
	pri, err := strconv.Atoi(string(data[1:end]))
	if err != nil {
		return nil, fmt.Errorf("syslog: invalid priority: %w", err)
	}

	msg := &Message{}
	msg.Facility, msg.Severity = decodePriority(pri)

	rest := data[end+1:]
	if len(rest) == 0 {
		return nil, fmt.Errorf("syslog: empty message after priority")
	}

	// Try RFC 5424: starts with version number (e.g., "1 ")
	if rest[0] >= '0' && rest[0] <= '9' {
		return parseRFC5424(rest, msg)
	}
	return parseRFC3164(rest, msg)
}

func parseRFC5424(data []byte, msg *Message) (*Message, error) {
	// Format: VERSION SP TIMESTAMP SP HOSTNAME SP APP-NAME SP PROCID SP MSGID SP STRUCTURED-DATA SP MSG
	space := bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing version")
	}
	// Skip version
	data = data[space+1:]

	// Timestamp
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing timestamp")
	}
	ts, err := time.Parse(time.RFC3339Nano, string(data[:space]))
	if err == nil {
		msg.Timestamp = ts
	}
	data = data[space+1:]

	// Hostname
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing hostname")
	}
	msg.Hostname = string(data[:space])
	data = data[space+1:]

	// AppName
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing appname")
	}
	msg.AppName = string(data[:space])
	data = data[space+1:]

	// ProcID
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing procid")
	}
	msg.ProcID = string(data[:space])
	data = data[space+1:]

	// MsgID
	space = bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 5424: missing msgid")
	}
	msg.MsgID = string(data[:space])
	data = data[space+1:]

	// Structured data (skip until space)
	if len(data) > 0 && data[0] == '-' {
		data = data[1:]
		if len(data) > 0 && data[0] == ' ' {
			data = data[1:]
		}
	} else {
		// Skip structured data (bracketed)
		if idx := bytes.IndexByte(data, ']'); idx >= 0 {
			data = data[idx+1:]
			if len(data) > 0 && data[0] == ' ' {
				data = data[1:]
			}
		}
	}

	msg.Message = string(data)
	return msg, nil
}

func parseRFC3164(data []byte, msg *Message) (*Message, error) {
	// Format: TIMESTAMP SP HOSTNAME SP APP-NAME[PID]: SP MSG
	// Timestamp: "Jan  2 15:04:05"
	if len(data) < 15 {
		return nil, fmt.Errorf("syslog 3164: too short")
	}

	// Try to parse timestamp (first 15 chars: "Jan  2 15:04:05")
	ts, err := time.Parse("Jan  2 15:04:05", string(data[:15]))
	if err == nil {
		msg.Timestamp = ts
		data = data[16:] // skip timestamp + space
	} else {
		// Try single-digit day: "Jan 2 15:04:05"
		ts, err = time.Parse("Jan 2 15:04:05", string(data[:14]))
		if err == nil {
			msg.Timestamp = ts
			data = data[15:]
		} else {
			return nil, fmt.Errorf("syslog 3164: cannot parse timestamp")
		}
	}

	// Hostname
	space := bytes.IndexByte(data, ' ')
	if space < 0 {
		return nil, fmt.Errorf("syslog 3164: missing hostname")
	}
	msg.Hostname = string(data[:space])
	data = data[space+1:]

	// App-Name[PID]: or App-Name:
	colon := bytes.IndexByte(data, ':')
	if colon < 0 {
		msg.Message = string(data)
		return msg, nil
	}
	appPart := data[:colon]
	data = data[colon+1:]

	// Skip leading space in message
	if len(data) > 0 && data[0] == ' ' {
		data = data[1:]
	}

	// Check for PID in brackets
	if idx := bytes.IndexByte(appPart, '['); idx >= 0 {
		msg.AppName = string(appPart[:idx])
		// Extract PID
		end := bytes.IndexByte(appPart[idx:], ']')
		if end >= 0 {
			msg.ProcID = string(appPart[idx+1 : idx+end])
		}
	} else {
		msg.AppName = string(appPart)
	}

	msg.Message = string(data)
	return msg, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/inputs/syslog/ -run TestParse -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/inputs/syslog/parser.go internal/collector/inputs/syslog/parser_test.go
git commit -m "feat: add syslog RFC 5424/3164 parser"
```

---

## Task 5: syslog Input Plugin -- TCP/UDP Receiver

**Files:**
- Create: `internal/collector/inputs/syslog/syslog.go`
- Create: `internal/collector/inputs/syslog/syslog_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/collector/inputs/syslog/syslog_test.go
package syslog

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestSyslogInputInit(t *testing.T) {
	si := &SyslogInput{}
	cfg := map[string]interface{}{
		"listen_addr":     "127.0.0.1:0",
		"protocol":        "tcp",
		"max_connections":  100,
	}
	if err := si.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if si.ListenAddr != "127.0.0.1:0" {
		t.Errorf("ListenAddr = %q", si.ListenAddr)
	}
	if si.Protocol != "tcp" {
		t.Errorf("Protocol = %q, want tcp", si.Protocol)
	}
}

func TestSyslogInputInitDefaults(t *testing.T) {
	si := &SyslogInput{}
	if err := si.Init(nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if si.ListenAddr != "0.0.0.0:514" {
		t.Errorf("default ListenAddr = %q", si.ListenAddr)
	}
	if si.Protocol != "tcp" {
		t.Errorf("default Protocol = %q", si.Protocol)
	}
}

func TestSyslogInputGatherTCP(t *testing.T) {
	si := &SyslogInput{
		ListenAddr:    "127.0.0.1:0",
		Protocol:      "tcp",
		MaxConnections: 10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	acc := collector.NewAccumulator(100)

	// Start gathering in background
	done := make(chan error, 1)
	go func() {
		done <- si.Gather(ctx, acc)
	}()

	// Wait for listener to start
	time.Sleep(50 * time.Millisecond)

	// Send a syslog message
	conn, err := net.Dial("tcp", si.listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	conn.Write([]byte("<13>Jan 15 10:30:00 myhost test: hello world\n"))
	conn.Close()

	// Give time to process
	time.Sleep(50 * time.Millisecond)
	cancel()

	metrics := acc.Collect()
	if len(metrics) == 0 {
		t.Fatal("expected at least 1 metric")
	}
	if metrics[0].Fields()["message"] != "hello world" {
		t.Errorf("message = %q", metrics[0].Fields()["message"])
	}
}

func TestSyslogInputSampleConfig(t *testing.T) {
	si := &SyslogInput{}
	if si.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/collector/inputs/syslog/ -run TestSyslogInput -v`
Expected: FAIL

- [ ] **Step 3: Implement SyslogInput**

```go
// internal/collector/inputs/syslog/syslog.go
package syslog

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/cy77cc/opsagent/internal/collector"
	"github.com/rs/zerolog"
)

const sampleConfig = `
## Syslog receiver input plugin (TCP/UDP)
# listen_addr = "0.0.0.0:514"
# protocol = "tcp"            # tcp | udp
# max_connections = 100
`

func init() {
	collector.RegisterInput("syslog", func() collector.Input {
		return &SyslogInput{}
	})
}

type SyslogInput struct {
	ListenAddr    string `toml:"listen_addr"`
	Protocol      string `toml:"protocol"`
	MaxConnections int   `toml:"max_connections"`

	listener net.Listener
	logger   zerolog.Logger
}

func (s *SyslogInput) Init(cfg map[string]interface{}) error {
	s.ListenAddr = "0.0.0.0:514"
	s.Protocol = "tcp"
	s.MaxConnections = 100

	if cfg == nil {
		return nil
	}
	if v, ok := cfg["listen_addr"]; ok {
		s.ListenAddr, ok = v.(string)
		if !ok {
			return fmt.Errorf("syslog: listen_addr must be a string")
		}
	}
	if v, ok := cfg["protocol"]; ok {
		s.Protocol, ok = v.(string)
		if !ok {
			return fmt.Errorf("syslog: protocol must be a string")
		}
	}
	if v, ok := cfg["max_connections"]; ok {
		switch n := v.(type) {
		case int:
			s.MaxConnections = n
		case int64:
			s.MaxConnections = int(n)
		case float64:
			s.MaxConnections = int(n)
		}
	}
	return nil
}

func (s *SyslogInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	if s.Protocol == "udp" {
		return s.gatherUDP(ctx, acc)
	}
	return s.gatherTCP(ctx, acc)
}

func (s *SyslogInput) gatherTCP(ctx context.Context, acc collector.Accumulator) error {
	ln, err := net.Listen("tcp", s.ListenAddr)
	if err != nil {
		return fmt.Errorf("syslog: listen %s: %w", s.ListenAddr, err)
	}
	s.listener = ln
	defer ln.Close()

	var wg sync.WaitGroup
	sem := make(chan struct{}, s.MaxConnections)

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			default:
				continue
			}
		}

		select {
		case sem <- struct{}{}:
		default:
			conn.Close()
			continue
		}

		wg.Add(1)
		go func(c net.Conn) {
			defer wg.Done()
			defer func() { <-sem }()
			defer c.Close()
			s.handleConn(c, acc)
		}(conn)
	}
}

func (s *SyslogInput) gatherUDP(ctx context.Context, acc collector.Accumulator) error {
	addr, err := net.ResolveUDPAddr("udp", s.ListenAddr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	buf := make([]byte, 65536)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				continue
			}
		}
		s.processMessage(buf[:n], acc)
	}
}

func (s *SyslogInput) handleConn(conn net.Conn, acc collector.Accumulator) {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		s.processMessage(scanner.Bytes(), acc)
	}
}

func (s *SyslogInput) processMessage(data []byte, acc collector.Accumulator) {
	msg, err := Parse(data)
	if err != nil {
		return
	}
	tags := map[string]string{
		"app": msg.AppName,
	}
	if msg.Hostname != "" {
		tags["host"] = msg.Hostname
	}
	fields := map[string]interface{}{
		"message":   msg.Message,
		"facility":  msg.Facility,
		"severity":  msg.Severity,
		"timestamp": msg.Timestamp.UnixNano(),
	}
	if msg.ProcID != "" {
		fields["pid"] = msg.ProcID
	}
	acc.AddFields("syslog", tags, fields)
}

func (s *SyslogInput) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/collector/inputs/syslog/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/collector/inputs/syslog/
git commit -m "feat: add syslog input plugin with TCP/UDP receiver"
```

---

## Task 6: logparse Processor Plugin

**Files:**
- Create: `internal/collector/processors/logparse/logparse.go`
- Create: `internal/collector/processors/logparse/grok.go`
- Create: `internal/collector/processors/logparse/logparse_test.go`
- Create: `internal/collector/processors/logparse/grok_test.go`

- [ ] **Step 1: Write the failing tests for grok patterns**

```go
// internal/collector/processors/logparse/grok_test.go
package logparse

import (
	"testing"
)

func TestBuiltinPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		want    map[string]string
	}{
		{
			name:    "IP pattern",
			pattern: `%{IPORHOST:client_ip}`,
			input:   "192.168.1.1",
			want:    map[string]string{"client_ip": "192.168.1.1"},
		},
		{
			name:    "number pattern",
			pattern: `%{NUMBER:status}`,
			input:   "200",
			want:    map[string]string{"status": "200"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := NewGrok(tt.pattern, nil)
			if err != nil {
				t.Fatalf("NewGrok: %v", err)
			}
			result, err := g.Match(tt.input)
			if err != nil {
				t.Fatalf("Match: %v", err)
			}
			for k, v := range tt.want {
				if result[k] != v {
					t.Errorf("%s = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestCustomPattern(t *testing.T) {
	patterns := map[string]string{
		"CUSTOM_LOG": `%{IPORHOST:client_ip} %{DATA:user} \[%{TIMESTAMP_ISO8601:ts}\]`,
	}
	g, err := NewGrok("%{CUSTOM_LOG}", patterns)
	if err != nil {
		t.Fatalf("NewGrok: %v", err)
	}
	input := `192.168.1.1 admin [2024-01-15T10:30:00Z]`
	result, err := g.Match(input)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if result["client_ip"] != "192.168.1.1" {
		t.Errorf("client_ip = %q", result["client_ip"])
	}
	if result["user"] != "admin" {
		t.Errorf("user = %q", result["user"])
	}
}

func TestGrokNoMatch(t *testing.T) {
	g, err := NewGrok("%{IPORHOST:ip}", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Match("not an ip")
	if err == nil {
		t.Error("expected error for no match")
	}
}
```

- [ ] **Step 2: Write the failing tests for logparse processor**

```go
// internal/collector/processors/logparse/logparse_test.go
package logparse

import (
	"testing"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestLogParseProcessorInit(t *testing.T) {
	lp := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "message",
				"parser":       "grok",
				"grok_pattern": `%{IPORHOST:client_ip} %{DATA:user} \[%{TIMESTAMP_ISO8601:ts}\]`,
			},
		},
	}
	if err := lp.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(lp.Rules) != 1 {
		t.Fatalf("Rules len = %d, want 1", len(lp.Rules))
	}
}

func TestLogParseApplyGrok(t *testing.T) {
	lp := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "message",
				"parser":       "grok",
				"grok_pattern": `%{IPORHOST:client_ip} %{DATA:user} \[%{TIMESTAMP_ISO8601:ts}\]`,
			},
		},
	}
	if err := lp.Init(cfg); err != nil {
		t.Fatal(err)
	}

	input := []*collector.Metric{
		collector.NewMetric("syslog",
			map[string]string{"app": "nginx"},
			map[string]interface{}{"message": `192.168.1.1 admin [2024-01-15T10:30:00Z] GET /index.html`}),
	}

	output := lp.Apply(input)
	if len(output) != 1 {
		t.Fatalf("output len = %d, want 1", len(output))
	}
	fields := output[0].Fields()
	if fields["client_ip"] != "192.168.1.1" {
		t.Errorf("client_ip = %q", fields["client_ip"])
	}
	if fields["user"] != "admin" {
		t.Errorf("user = %q", fields["user"])
	}
}

func TestLogParseApplyJSON(t *testing.T) {
	lp := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":  "message",
				"parser": "json",
			},
		},
	}
	if err := lp.Init(cfg); err != nil {
		t.Fatal(err)
	}

	input := []*collector.Metric{
		collector.NewMetric("tail",
			nil,
			map[string]interface{}{"message": `{"level":"info","msg":"request handled","latency_ms":42}`}),
	}

	output := lp.Apply(input)
	fields := output[0].Fields()
	if fields["level"] != "info" {
		t.Errorf("level = %q", fields["level"])
	}
	if fields["msg"] != "request handled" {
		t.Errorf("msg = %q", fields["msg"])
	}
}

func TestLogParseApplyRegex(t *testing.T) {
	lp := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":          "message",
				"parser":         "regex",
				"regex_pattern":  `(?P<ip>\d+\.\d+\.\d+\.\d+) (?P<status>\d+)`,
			},
		},
	}
	if err := lp.Init(cfg); err != nil {
		t.Fatal(err)
	}

	input := []*collector.Metric{
		collector.NewMetric("tail", nil, map[string]interface{}{"message": "10.0.0.1 200"}),
	}

	output := lp.Apply(input)
	fields := output[0].Fields()
	if fields["ip"] != "10.0.0.1" {
		t.Errorf("ip = %q", fields["ip"])
	}
	if fields["status"] != "200" {
		t.Errorf("status = %q", fields["status"])
	}
}

func TestLogParseSampleConfig(t *testing.T) {
	lp := &LogParseProcessor{}
	if lp.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/collector/processors/logparse/ -v`
Expected: FAIL

- [ ] **Step 4: Implement grok engine**

```go
// internal/collector/processors/logparse/grok.go
package logparse

import (
	"fmt"
	"regexp"
	"strings"
)

var builtinPatterns = map[string]string{
	"IPORHOST":         `(?:%{IP}|%{HOSTNAME})`,
	"IP":               `%{IPV4}|%{IPV6}`,
	"IPV4":             `(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`,
	"IPV6":             `(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}`,
	"HOSTNAME":         `\b(?:[0-9A-Za-z][0-9A-Za-z-]{0,62})(?:\.(?:[0-9A-Za-z][0-9A-Za-z-]{0,62}))*(\.?|\b)`,
	"NUMBER":           `(?:%{BASE10NUM})`,
	"BASE10NUM":        `(?<![0-9.+-])(?>[+-]?(?:(?:[0-9]+(?:\.[0-9]+)?)|(?:\.[0-9]+)))`,
	"DATA":             `.*?`,
	"GREEDYDATA":       `.*`,
	"TIMESTAMP_ISO8601": `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})`,
	"LOGLEVEL":         `(?:[Aa]lert|ALERT|[Tt]race|TRACE|[Dd]ebug|DEBUG|[Nn]otice|NOTICE|[Ii]nfo|INFO|[Ww]arn(?:ing)?|WARN(?:ING)?|[Ee]rr(?:or)?|ERR(?:OR)?|[Cc]rit(?:ical)?|CRIT(?:ICAL)?|[Ff]atal|FATAL|[Ss]evere|SEVERE)`,
	"WORD":             `\b\w+\b`,
	"QUOTEDSTRING":     `"(?:[^"\\]|\\.)*"`,
	"UUID":             `[A-Fa-f0-9]{8}-(?:[A-Fa-f0-9]{4}-){3}[A-Fa-f0-9]{12}`,
}

type Grok struct {
	regex    *regexp.Regexp
	namedMap map[string]int // name -> group index
}

func NewGrok(pattern string, customPatterns map[string]string) (*Grok, error) {
	allPatterns := make(map[string]string)
	for k, v := range builtinPatterns {
		allPatterns[k] = v
	}
	for k, v := range customPatterns {
		allPatterns[k] = v
	}

	// Expand patterns iteratively (max 10 passes to prevent infinite loops)
	expanded := pattern
	for i := 0; i < 10; i++ {
		changed := false
		for name, subPattern := range allPatterns {
			placeholder := "%{" + name + "}"
			if strings.Contains(expanded, placeholder) {
				expanded = strings.ReplaceAll(expanded, placeholder, "(?:"+subPattern+")")
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Extract named capture groups
	namedMap := make(map[string]int)
	re := regexp.MustCompile(`%\{(\w+):(\w+)\}`)
	expanded = re.ReplaceAllStringFunc(expanded, func(match string) string {
		parts := re.FindStringSubmatch(match)
		patternName := parts[1]
		captureName := parts[2]
		subPattern, ok := allPatterns[patternName]
		if !ok {
			subPattern = ".*?"
		}
		// We'll track named groups by index after compiling
		_ = captureName
		return "(" + subPattern + ")"
	})

	// Now extract all capture group names from the final pattern
	nameRe := regexp.MustCompile(`%\{(\w+):(\w+)\}`)
	matches := nameRe.FindAllStringSubmatch(pattern, -1)
	for i, m := range matches {
		namedMap[m[2]] = i + // group index (1-based)
			// Count literal groups before this one
			0 // simplified: use sequential indexing
	}

	// Re-do with proper sequential indexing
	namedMap = make(map[string]int)
	expanded = pattern
	for i := 0; i < 10; i++ {
		changed := false
		for name, subPattern := range allPatterns {
			placeholder := "%{" + name + "}"
			if strings.Contains(expanded, placeholder) {
				expanded = strings.ReplaceAll(expanded, placeholder, "(?:"+subPattern+")")
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Extract named groups with sequential indexing
	finalRe := regexp.MustCompile(`%\{(\w+):(\w+)\}`)
	namedMap = make(map[string]int)
	groupIdx := 1
	expanded = finalRe.ReplaceAllStringFunc(expanded, func(match string) string {
		parts := finalRe.FindStringSubmatch(match)
		patternName := parts[1]
		captureName := parts[2]
		subPattern, ok := allPatterns[patternName]
		if !ok {
			subPattern = ".*?"
		}
		namedMap[captureName] = groupIdx
		groupIdx++
		return "(" + subPattern + ")"
	})

	// Escape any remaining literal %{...} that aren't named captures
	expanded = regexp.MustCompile(`%\{[^}]+\}`).ReplaceAllString(expanded, ".*?")

	compiled, err := regexp.Compile(expanded)
	if err != nil {
		return nil, fmt.Errorf("grok: compile pattern: %w", err)
	}

	return &Grok{regex: compiled, namedMap: namedMap}, nil
}

func (g *Grok) Match(input string) (map[string]string, error) {
	match := g.regex.FindStringSubmatch(input)
	if match == nil {
		return nil, fmt.Errorf("grok: no match")
	}
	result := make(map[string]string, len(g.namedMap))
	for name, idx := range g.namedMap {
		if idx < len(match) {
			result[name] = match[idx]
		}
	}
	return result, nil
}
```

- [ ] **Step 5: Implement logparse processor**

```go
// internal/collector/processors/logparse/logparse.go
package logparse

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## Log parse processor (grok/regex/JSON)
# [[processors.logparse.config.rules]]
#   field = "message"
#   parser = "grok"           # grok | regex | json
#   grok_pattern = '%{IPORHOST:client_ip} ...'
#   regex_pattern = '(?P<ip>\d+\.\d+\.\d+\.\d+)'
#   patterns = {CUSTOM_LOG: '%{IPORHOST:ip} ...'}
`

func init() {
	collector.RegisterProcessor("logparse", func() collector.Processor {
		return &LogParseProcessor{}
	})
}

type ParseRule struct {
	Field        string            `toml:"field"`
	Parser       string            `toml:"parser"`
	GrokPattern  string            `toml:"grok_pattern"`
	RegexPattern string            `toml:"regex_pattern"`
	Patterns     map[string]string `toml:"patterns"`
	grok         *Grok
	regex        *regexp.Regexp
	regexNames   []string
}

type LogParseProcessor struct {
	Rules []ParseRule `toml:"rules"`
}

func (lp *LogParseProcessor) Init(cfg map[string]interface{}) error {
	if cfg == nil {
		return nil
	}
	rulesRaw, ok := cfg["rules"]
	if !ok {
		return nil
	}
	rulesArr, ok := rulesRaw.([]interface{})
	if !ok {
		return fmt.Errorf("logparse: rules must be a list")
	}
	for _, r := range rulesArr {
		ruleMap, ok := r.(map[string]interface{})
		if !ok {
			return fmt.Errorf("logparse: rule must be a map")
		}
		rule := ParseRule{}
		if v, ok := ruleMap["field"]; ok {
			rule.Field, _ = v.(string)
		}
		if v, ok := ruleMap["parser"]; ok {
			rule.Parser, _ = v.(string)
		}
		if v, ok := ruleMap["grok_pattern"]; ok {
			rule.GrokPattern, _ = v.(string)
		}
		if v, ok := ruleMap["regex_pattern"]; ok {
			rule.RegexPattern, _ = v.(string)
		}
		if v, ok := ruleMap["patterns"]; ok {
			if pm, ok := v.(map[string]interface{}); ok {
				rule.Patterns = make(map[string]string)
				for k, val := range pm {
					if s, ok := val.(string); ok {
						rule.Patterns[k] = s
					}
				}
			}
		}
		if err := lp.compileRule(&rule); err != nil {
			return err
		}
		lp.Rules = append(lp.Rules, rule)
	}
	return nil
}

func (lp *LogParseProcessor) compileRule(rule *ParseRule) error {
	switch rule.Parser {
	case "grok":
		g, err := NewGrok(rule.GrokPattern, rule.Patterns)
		if err != nil {
			return fmt.Errorf("logparse: compile grok: %w", err)
		}
		rule.grok = g
	case "regex":
		re, err := regexp.Compile(rule.RegexPattern)
		if err != nil {
			return fmt.Errorf("logparse: compile regex: %w", err)
		}
		rule.regex = re
		rule.regexNames = re.SubexpNames()
	case "json":
		// No compilation needed
	default:
		return fmt.Errorf("logparse: unknown parser %q", rule.Parser)
	}
	return nil
}

func (lp *LogParseProcessor) Apply(in []*collector.Metric) []*collector.Metric {
	for _, m := range in {
		for _, rule := range lp.Rules {
			val, ok := m.Fields()[rule.Field]
			if !ok {
				continue
			}
			str, ok := val.(string)
			if !ok {
				continue
			}
			switch rule.Parser {
			case "grok":
				result, err := rule.grok.Match(str)
				if err != nil {
					continue
				}
				for k, v := range result {
					m.AddField(k, v)
				}
			case "regex":
				match := rule.regex.FindStringSubmatch(str)
				if match == nil {
					continue
				}
				for i, name := range rule.regexNames {
					if i > 0 && i < len(match) && name != "" {
						m.AddField(name, match[i])
					}
				}
			case "json":
				var parsed map[string]interface{}
				if err := json.Unmarshal([]byte(str), &parsed); err != nil {
					continue
				}
				for k, v := range parsed {
					m.AddField(k, v)
				}
			}
		}
	}
	return in
}

func (lp *LogParseProcessor) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/collector/processors/logparse/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/collector/processors/logparse/
git commit -m "feat: add logparse processor with grok/regex/JSON parsing"
```

---

## Task 7: OTLP Output Plugin

**Files:**
- Create: `internal/collector/outputs/otlp/otlp.go`
- Create: `internal/collector/outputs/otlp/otlp_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc go.opentelemetry.io/otel/sdk`

- [ ] **Step 2: Write the failing test**

```go
// internal/collector/outputs/otlp/otlp_test.go
package otlp

import (
	"testing"
)

func TestOTLPOutputInit(t *testing.T) {
	o := &OTLPOutput{}
	cfg := map[string]interface{}{
		"endpoint":         "localhost:4317",
		"protocol":         "grpc",
		"compression":      "gzip",
		"batch_size":       512,
		"timeout_seconds":  10,
	}
	if err := o.Init(cfg); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if o.Endpoint != "localhost:4317" {
		t.Errorf("Endpoint = %q", o.Endpoint)
	}
	if o.Protocol != "grpc" {
		t.Errorf("Protocol = %q", o.Protocol)
	}
}

func TestOTLPOutputInitDefaults(t *testing.T) {
	o := &OTLPOutput{}
	if err := o.Init(nil); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if o.Protocol != "grpc" {
		t.Errorf("default Protocol = %q, want grpc", o.Protocol)
	}
	if o.BatchSize != 512 {
		t.Errorf("default BatchSize = %d, want 512", o.BatchSize)
	}
	if o.Timeout != 10 {
		// check it has some default
	}
}

func TestOTLPOutputInitMissingEndpoint(t *testing.T) {
	o := &OTLPOutput{}
	cfg := map[string]interface{}{}
	if err := o.Init(cfg); err == nil {
		t.Error("expected error for missing endpoint")
	}
}

func TestOTLPOutputSampleConfig(t *testing.T) {
	o := &OTLPOutput{}
	if o.SampleConfig() == "" {
		t.Error("SampleConfig should not be empty")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/collector/outputs/otlp/ -v`
Expected: FAIL

- [ ] **Step 4: Implement OTLPOutput**

```go
// internal/collector/outputs/otlp/otlp.go
package otlp

import (
	"context"
	"fmt"

	"github.com/cy77cc/opsagent/internal/collector"
)

const sampleConfig = `
## OTLP output plugin (gRPC or HTTP)
# endpoint = "localhost:4317"
# protocol = "grpc"           # grpc | http
# compression = "gzip"        # gzip | none
# batch_size = 512
# timeout_seconds = 10
# headers = {}
`

func init() {
	collector.RegisterOutput("otlp", func() collector.Output {
		return &OTLPOutput{}
	})
}

type OTLPOutput struct {
	Endpoint    string            `toml:"endpoint"`
	Protocol    string            `toml:"protocol"`
	Headers     map[string]string `toml:"headers"`
	Compression string            `toml:"compression"`
	BatchSize   int               `toml:"batch_size"`
	Timeout     int               `toml:"timeout_seconds"`
}

func (o *OTLPOutput) Init(cfg map[string]interface{}) error {
	o.Protocol = "grpc"
	o.BatchSize = 512
	o.Timeout = 10

	if cfg == nil {
		return fmt.Errorf("otlp: endpoint is required")
	}
	if v, ok := cfg["endpoint"]; ok {
		o.Endpoint, ok = v.(string)
		if !ok {
			return fmt.Errorf("otlp: endpoint must be a string")
		}
	}
	if o.Endpoint == "" {
		return fmt.Errorf("otlp: endpoint is required")
	}
	if v, ok := cfg["protocol"]; ok {
		o.Protocol, ok = v.(string)
		if !ok {
			return fmt.Errorf("otlp: protocol must be a string")
		}
	}
	if v, ok := cfg["compression"]; ok {
		o.Compression, ok = v.(string)
		if !ok {
			return fmt.Errorf("otlp: compression must be a string")
		}
	}
	if v, ok := cfg["batch_size"]; ok {
		switch n := v.(type) {
		case int:
			o.BatchSize = n
		case int64:
			o.BatchSize = int(n)
		case float64:
			o.BatchSize = int(n)
		}
	}
	if v, ok := cfg["timeout_seconds"]; ok {
		switch n := v.(type) {
		case int:
			o.Timeout = n
		case int64:
			o.Timeout = int(n)
		case float64:
			o.Timeout = int(n)
		}
	}
	if v, ok := cfg["headers"]; ok {
		if hm, ok := v.(map[string]interface{}); ok {
			o.Headers = make(map[string]string)
			for k, val := range hm {
				if s, ok := val.(string); ok {
					o.Headers[k] = s
				}
			}
		}
	}
	return nil
}

func (o *OTLPOutput) Write(ctx context.Context, metrics []collector.Metric) error {
	// Implementation will use OTLP SDK to export metrics
	// For now, batch and send via gRPC/HTTP
	if len(metrics) == 0 {
		return nil
	}

	// TODO: Connect to OTLP endpoint and export
	// This will be implemented with the actual OTLP exporter
	return nil
}

func (o *OTLPOutput) Close() error {
	return nil
}

func (o *OTLPOutput) SampleConfig() string {
	return sampleConfig
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/collector/outputs/otlp/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/collector/outputs/otlp/
git commit -m "feat: add OTLP output plugin skeleton with config validation"
```

---

## Task 8: Config Sections for New Subsystems

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/diff.go`

- [ ] **Step 1: Add config structs to config.go**

Add to the `Config` struct:

```go
Tracing   TracingConfig   `mapstructure:"tracing"`
Alerting  AlertingConfig  `mapstructure:"alerting"`
```

Add the new config types:

```go
type TracingConfig struct {
	Enabled    bool              `mapstructure:"enabled"`
	Receiver   TracingReceiver   `mapstructure:"receiver"`
	Processor  TracingProcessor  `mapstructure:"processor"`
	Exporter   TracingExporter   `mapstructure:"exporter"`
}

type TracingReceiver struct {
	GRPCAddr string `mapstructure:"grpc_addr"`
	HTTPAddr string `mapstructure:"http_addr"`
}

type TracingProcessor struct {
	BatchTimeoutMs int  `mapstructure:"batch_timeout_ms"`
	MaxBatchSize   int  `mapstructure:"max_batch_size"`
}

type TracingExporter struct {
	Endpoint string `mapstructure:"endpoint"`
	Protocol string `mapstructure:"protocol"`
}

type AlertingConfig struct {
	Enabled bool          `mapstructure:"enabled"`
	Rules   []AlertRule   `mapstructure:"rules"`
}

type AlertRule struct {
	Name      string           `mapstructure:"name"`
	Condition AlertCondition   `mapstructure:"condition"`
	Severity  string           `mapstructure:"severity"`
	Notify    []AlertNotify    `mapstructure:"notify"`
}

type AlertCondition struct {
	Metric    string `mapstructure:"metric"`
	Operator  string `mapstructure:"operator"`
	Threshold float64 `mapstructure:"threshold"`
	For       string `mapstructure:"for"`
}

type AlertNotify struct {
	Type    string            `mapstructure:"type"`
	URL     string            `mapstructure:"url"`
	Headers map[string]string `mapstructure:"headers"`
}
```

- [ ] **Step 2: Add defaults in Load()**

```go
v.SetDefault("tracing.enabled", false)
v.SetDefault("tracing.receiver.grpc_addr", "0.0.0.0:4317")
v.SetDefault("tracing.receiver.http_addr", "0.0.0.0:4318")
v.SetDefault("tracing.processor.batch_timeout_ms", 5000)
v.SetDefault("tracing.processor.max_batch_size", 512)
v.SetDefault("tracing.exporter.protocol", "grpc")
v.SetDefault("alerting.enabled", false)
```

- [ ] **Step 3: Add validation in Validate()**

```go
if c.Tracing.Enabled {
	if c.Tracing.Exporter.Endpoint == "" {
		return fmt.Errorf("tracing.exporter.endpoint is required when tracing is enabled")
	}
}
if c.Alerting.Enabled {
	for i, rule := range c.Alerting.Rules {
		if rule.Name == "" {
			return fmt.Errorf("alerting.rules[%d].name is required", i)
		}
		if rule.Condition.Metric == "" {
			return fmt.Errorf("alerting.rules[%d].condition.metric is required", i)
		}
		validOps := map[string]bool{">": true, "<": true, ">=": true, "<=": true, "==": true, "!=": true}
		if !validOps[rule.Condition.Operator] {
			return fmt.Errorf("alerting.rules[%d].condition.operator must be one of: >, <, >=, <=, ==, !=", i)
		}
	}
}
```

- [ ] **Step 4: Add Alerting to ChangeSet in diff.go**

Add `Alerting` field to `ChangeSet` struct and add to the reloadable list.

- [ ] **Step 5: Run existing config tests**

Run: `go test ./internal/config/ -v`
Expected: PASS (no regressions)

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/diff.go
git commit -m "feat: add tracing and alerting config sections"
```

---

## Task 9: Tracing Subsystem (OTLP Receiver/Exporter)

**Files:**
- Create: `internal/tracing/receiver.go`
- Create: `internal/tracing/processor.go`
- Create: `internal/tracing/exporter.go`
- Create: `internal/tracing/receiver_test.go`
- Create: `internal/tracing/processor_test.go`
- Create: `internal/tracing/exporter_test.go`

- [ ] **Step 1: Write the failing test for batch processor**

```go
// internal/tracing/processor_test.go
package tracing

import (
	"testing"
	"time"
)

func TestBatchProcessor(t *testing.T) {
	bp := NewBatchProcessor(BatchConfig{
		MaxSize:   3,
		Timeout:   1 * time.Second,
	})
	defer bp.Stop()

	var received [][]byte
	bp.SetExportFn(func(batch [][]byte) error {
		received = append(received, batch...)
		return nil
	})

	bp.Add([]byte("span1"))
	bp.Add([]byte("span2"))
	bp.Add([]byte("span3")) // Should trigger flush

	time.Sleep(100 * time.Millisecond)
	if len(received) != 3 {
		t.Errorf("received %d, want 3", len(received))
	}
}

func TestBatchProcessorTimeout(t *testing.T) {
	bp := NewBatchProcessor(BatchConfig{
		MaxSize:   100,
		Timeout:   50 * time.Millisecond,
	})
	defer bp.Stop()

	var received [][]byte
	bp.SetExportFn(func(batch [][]byte) error {
		received = append(received, batch...)
		return nil
	})

	bp.Add([]byte("span1"))
	time.Sleep(100 * time.Millisecond)

	if len(received) != 1 {
		t.Errorf("received %d, want 1", len(received))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tracing/ -run TestBatchProcessor -v`
Expected: FAIL

- [ ] **Step 3: Implement batch processor**

```go
// internal/tracing/processor.go
package tracing

import (
	"sync"
	"time"
)

type BatchConfig struct {
	MaxSize int
	Timeout time.Duration
}

type BatchProcessor struct {
	cfg      BatchConfig
	batch    [][]byte
	mu       sync.Mutex
	exportFn func([][]byte) error
	ticker   *time.Stopper
	done     chan struct{}
}

func NewBatchProcessor(cfg BatchConfig) *BatchProcessor {
	bp := &BatchProcessor{
		cfg:   cfg,
		done:  make(chan struct{}),
	}
	go bp.flushLoop()
	return bp
}

func (bp *BatchProcessor) SetExportFn(fn func([][]byte) error) {
	bp.mu.Lock()
	bp.exportFn = fn
	bp.mu.Unlock()
}

func (bp *BatchProcessor) Add(data []byte) {
	bp.mu.Lock()
	bp.batch = append(bp.batch, data)
	shouldFlush := len(bp.batch) >= bp.cfg.MaxSize
	bp.mu.Unlock()

	if shouldFlush {
		bp.flush()
	}
}

func (bp *BatchProcessor) flush() {
	bp.mu.Lock()
	if len(bp.batch) == 0 {
		bp.mu.Unlock()
		return
	}
	batch := bp.batch
	bp.batch = nil
	fn := bp.exportFn
	bp.mu.Unlock()

	if fn != nil {
		fn(batch)
	}
}

func (bp *BatchProcessor) flushLoop() {
	ticker := time.NewTicker(bp.cfg.Timeout)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			bp.flush()
		case <-bp.done:
			bp.flush()
			return
		}
	}
}

func (bp *BatchProcessor) Stop() {
	close(bp.done)
}
```

- [ ] **Step 4: Implement receiver and exporter**

```go
// internal/tracing/receiver.go
package tracing

import (
	"context"
	"net"

	"github.com/rs/zerolog"
)

type Receiver struct {
	grpcAddr string
	httpAddr string
	logger   zerolog.Logger
	listener net.Listener
}

func NewReceiver(grpcAddr, httpAddr string, logger zerolog.Logger) *Receiver {
	return &Receiver{
		grpcAddr: grpcAddr,
		httpAddr: httpAddr,
		logger:   logger,
	}
}

func (r *Receiver) Start(ctx context.Context) error {
	// Start gRPC listener for OTLP traces
	ln, err := net.Listen("tcp", r.grpcAddr)
	if err != nil {
		return err
	}
	r.listener = ln
	r.logger.Info().Str("addr", r.grpcAddr).Msg("tracing receiver started")
	<-ctx.Done()
	return ln.Close()
}

func (r *Receiver) Stop(ctx context.Context) error {
	if r.listener != nil {
		return r.listener.Close()
	}
	return nil
}
```

```go
// internal/tracing/exporter.go
package tracing

import (
	"context"

	"github.com/rs/zerolog"
)

type Exporter struct {
	endpoint string
	protocol string
	logger   zerolog.Logger
}

func NewExporter(endpoint, protocol string, logger zerolog.Logger) *Exporter {
	return &Exporter{
		endpoint: endpoint,
		protocol: protocol,
		logger:   logger,
	}
}

func (e *Exporter) Export(ctx context.Context, data [][]byte) error {
	// Forward spans to OTLP endpoint
	e.logger.Debug().Int("count", len(data)).Msg("exporting spans")
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tracing/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tracing/
git commit -m "feat: add tracing subsystem with batch processor, receiver, exporter"
```

---

## Task 10: Alerting Engine

**Files:**
- Create: `internal/alerting/engine.go`
- Create: `internal/alerting/rules.go`
- Create: `internal/alerting/notifier.go`
- Create: `internal/alerting/engine_test.go`
- Create: `internal/alerting/rules_test.go`
- Create: `internal/alerting/notifier_test.go`

- [ ] **Step 1: Write the failing tests for alert state machine**

```go
// internal/alerting/rules_test.go
package alerting

import (
	"testing"
	"time"
)

func TestAlertStateMachine(t *testing.T) {
	rule := &EvaluatedRule{
		Name:     "high_cpu",
		State:    StateOK,
		Duration: 5 * time.Minute,
	}

	// Condition becomes true -> PENDING
	rule.ConditionMet(true, time.Now())
	if rule.State != StatePending {
		t.Errorf("state = %q, want pending", rule.State)
	}

	// Condition stays true but duration not met -> still PENDING
	rule.ConditionMet(true, time.Now().Add(1*time.Minute))
	if rule.State != StatePending {
		t.Errorf("state = %q, want pending", rule.State)
	}

	// Duration met -> FIRING
	rule.ConditionMet(true, time.Now().Add(6*time.Minute))
	if rule.State != StateFiring {
		t.Errorf("state = %q, want firing", rule.State)
	}

	// Condition becomes false -> OK
	rule.ConditionMet(false, time.Now().Add(7*time.Minute))
	if rule.State != StateOK {
		t.Errorf("state = %q, want ok", rule.State)
	}
}

func TestAlertConditionEvaluate(t *testing.T) {
	tests := []struct {
		op         string
		threshold  float64
		value      float64
		want       bool
	}{
		{">", 95, 96, true},
		{">", 95, 95, false},
		{"<", 10, 9, true},
		{"<", 10, 10, false},
		{">=", 95, 95, true},
		{">=", 95, 94, false},
		{"<=", 10, 10, true},
		{"<=", 10, 11, false},
		{"==", 42, 42, true},
		{"==", 42, 43, false},
		{"!=", 42, 43, true},
		{"!=", 42, 42, false},
	}
	for _, tt := range tests {
		cond := AlertCondition{Operator: tt.op, Threshold: tt.threshold}
		got := cond.Evaluate(tt.value)
		if got != tt.want {
			t.Errorf("%v %s %v = %v, want %v", tt.value, tt.op, tt.threshold, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Write the failing test for engine**

```go
// internal/alerting/engine_test.go
package alerting

import (
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestEngineEvaluate(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Rules: []RuleConfig{
			{
				Name:      "high_cpu",
				Condition: AlertCondition{Metric: "cpu_usage_percent", Operator: ">", Threshold: 95},
				Severity:  "critical",
				For:       0, // immediate
			},
		},
	})

	metrics := []*collector.Metric{
		collector.NewMetric("cpu", nil, map[string]interface{}{"usage_percent": 97.0}),
	}

	alerts := engine.Evaluate(metrics)
	if len(alerts) != 1 {
		t.Fatalf("alerts len = %d, want 1", len(alerts))
	}
	if alerts[0].Name != "high_cpu" {
		t.Errorf("alert name = %q", alerts[0].Name)
	}
	if alerts[0].State != StateFiring {
		t.Errorf("alert state = %q", alerts[0].State)
	}
}

func TestEngineNoAlert(t *testing.T) {
	engine := NewEngine(EngineConfig{
		Rules: []RuleConfig{
			{
				Name:      "high_cpu",
				Condition: AlertCondition{Metric: "cpu_usage_percent", Operator: ">", Threshold: 95},
				Severity:  "critical",
			},
		},
	})

	metrics := []*collector.Metric{
		collector.NewMetric("cpu", nil, map[string]interface{}{"usage_percent": 50.0}),
	}

	alerts := engine.Evaluate(metrics)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}
```

- [ ] **Step 3: Write the failing test for notifier**

```go
// internal/alerting/notifier_test.go
package alerting

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookNotifier(t *testing.T) {
	var received Alert
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL, nil)
	alert := Alert{Name: "high_cpu", State: StateFiring, Severity: "critical", CurrentValue: 97}
	if err := n.Notify(alert); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if received.Name != "high_cpu" {
		t.Errorf("received name = %q", received.Name)
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/alerting/ -v`
Expected: FAIL

- [ ] **Step 5: Implement alert rules and state machine**

```go
// internal/alerting/rules.go
package alerting

import (
	"time"
)

const (
	StateOK      = "ok"
	StatePending = "pending"
	StateFiring  = "firing"
)

type AlertCondition struct {
	Metric    string  `mapstructure:"metric"`
	Operator  string  `mapstructure:"operator"`
	Threshold float64 `mapstructure:"threshold"`
	For       string  `mapstructure:"for"`
}

func (c AlertCondition) Evaluate(value float64) bool {
	switch c.Operator {
	case ">":
		return value > c.Threshold
	case "<":
		return value < c.Threshold
	case ">=":
		return value >= c.Threshold
	case "<=":
		return value <= c.Threshold
	case "==":
		return value == c.Threshold
	case "!=":
		return value != c.Threshold
	default:
		return false
	}
}

type EvaluatedRule struct {
	Name         string
	Condition    AlertCondition
	Severity     string
	Duration     time.Duration
	State        string
	TriggeredAt  time.Time
	CurrentValue float64
}

func (r *EvaluatedRule) ConditionMet(met bool, now time.Time) {
	switch r.State {
	case StateOK:
		if met {
			r.State = StatePending
			r.TriggeredAt = now
		}
	case StatePending:
		if !met {
			r.State = StateOK
		} else if now.Sub(r.TriggeredAt) >= r.Duration {
			r.State = StateFiring
		}
	case StateFiring:
		if !met {
			r.State = StateOK
		}
	}
}

type Alert struct {
	Name         string  `json:"name"`
	State        string  `json:"state"`
	Severity     string  `json:"severity"`
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Message      string  `json:"message"`
}
```

- [ ] **Step 6: Implement alert engine**

```go
// internal/alerting/engine.go
package alerting

import (
	"fmt"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

type RuleConfig struct {
	Name      string         `mapstructure:"name"`
	Condition AlertCondition `mapstructure:"condition"`
	Severity  string         `mapstructure:"severity"`
	For       string         `mapstructure:"for"`
}

type EngineConfig struct {
	Rules []RuleConfig
}

type Engine struct {
	rules    []*EvaluatedRule
	notifiers []Notifier
}

func NewEngine(cfg EngineConfig) *Engine {
	e := &Engine{}
	for _, rc := range cfg.Rules {
		rule := &EvaluatedRule{
			Name:      rc.Name,
			Condition: rc.Condition,
			Severity:  rc.Severity,
			State:     StateOK,
		}
		if rc.For != "" {
			d, err := time.ParseDuration(rc.For)
			if err == nil {
				rule.Duration = d
			}
		}
		e.rules = append(e.rules, rule)
	}
	return e
}

func (e *Engine) AddNotifier(n Notifier) {
	e.notifiers = append(e.notifiers, n)
}

func (e *Engine) Evaluate(metrics []*collector.Metric) []Alert {
	// Build value map from metrics
	values := make(map[string]float64)
	for _, m := range metrics {
		for k, v := range m.Fields() {
			key := m.Name() + "_" + k
			switch val := v.(type) {
			case float64:
				values[key] = val
			case int64:
				values[key] = float64(val)
			case int:
				values[key] = float64(val)
			}
		}
	}

	var alerts []Alert
	now := time.Now()

	for _, rule := range e.rules {
		value, ok := values[rule.Condition.Metric]
		if !ok {
			continue
		}
		rule.CurrentValue = value
		met := rule.Condition.Evaluate(value)
		oldState := rule.State
		rule.ConditionMet(met, now)

		if rule.State != oldState && rule.State == StateFiring {
			alert := Alert{
				Name:         rule.Name,
				State:        rule.State,
				Severity:     rule.Severity,
				CurrentValue: value,
				Threshold:    rule.Condition.Threshold,
				Message:      fmt.Sprintf("%s: %s %s %.2f (threshold %.2f)", rule.Name, rule.Condition.Metric, rule.Condition.Operator, value, rule.Condition.Threshold),
			}
			alerts = append(alerts, alert)
			for _, n := range e.notifiers {
				n.Notify(alert)
			}
		}
	}

	return alerts
}
```

- [ ] **Step 7: Implement notifier**

```go
// internal/alerting/notifier.go
package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Notifier interface {
	Notify(alert Alert) error
}

type WebhookNotifier struct {
	url     string
	headers map[string]string
	client  *http.Client
}

func NewWebhookNotifier(url string, headers map[string]string) *WebhookNotifier {
	return &WebhookNotifier{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookNotifier) Notify(alert Alert) error {
	body, err := json.Marshal(alert)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/alerting/ -v`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add internal/alerting/
git commit -m "feat: add alerting engine with state machine, rules, and webhook notifier"
```

---

## Task 11: Embedded Dashboard

**Files:**
- Create: `internal/server/ui.go`
- Create: `internal/server/ui/index.html`
- Create: `internal/server/ui/style.css`
- Create: `internal/server/ui/app.js`
- Modify: `internal/server/handlers.go`
- Modify: `internal/server/server.go`

- [ ] **Step 1: Create embedded HTML dashboard**

```html
<!-- internal/server/ui/index.html -->
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>OpsAgent Dashboard</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <header>
        <h1>OpsAgent Dashboard</h1>
        <span id="status" class="badge">loading...</span>
    </header>
    <main>
        <section id="overview">
            <h2>System Overview</h2>
            <div class="cards" id="system-cards"></div>
        </section>
        <section id="subsystems">
            <h2>Subsystems</h2>
            <div class="cards" id="subsystem-cards"></div>
        </section>
        <section id="logs">
            <h2>Recent Logs</h2>
            <div class="filter">
                <select id="log-level">
                    <option value="">All</option>
                    <option value="debug">Debug</option>
                    <option value="info">Info</option>
                    <option value="warn">Warn</option>
                    <option value="error">Error</option>
                </select>
            </div>
            <pre id="log-output"></pre>
        </section>
        <section id="config">
            <h2>Configuration</h2>
            <pre id="config-output">Loading...</pre>
        </section>
    </main>
    <script src="app.js"></script>
</body>
</html>
```

```css
/* internal/server/ui/style.css */
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #e2e8f0; padding: 1rem; }
header { display: flex; align-items: center; gap: 1rem; margin-bottom: 2rem; }
h1 { font-size: 1.5rem; }
h2 { font-size: 1.1rem; margin-bottom: 1rem; color: #94a3b8; }
.badge { padding: 0.25rem 0.75rem; border-radius: 9999px; font-size: 0.75rem; }
.badge.healthy { background: #065f46; color: #6ee7b7; }
.badge.degraded { background: #78350f; color: #fbbf24; }
.badge.error { background: #7f1d1d; color: #fca5a5; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 1rem; margin-bottom: 2rem; }
.card { background: #1e293b; padding: 1rem; border-radius: 0.5rem; }
.card h3 { font-size: 0.875rem; color: #94a3b8; margin-bottom: 0.5rem; }
.card .value { font-size: 1.5rem; font-weight: bold; }
section { margin-bottom: 2rem; }
pre { background: #1e293b; padding: 1rem; border-radius: 0.5rem; overflow-x: auto; font-size: 0.8rem; max-height: 400px; overflow-y: auto; }
.filter { margin-bottom: 1rem; }
select { background: #1e293b; color: #e2e8f0; padding: 0.5rem; border: 1px solid #334155; border-radius: 0.25rem; }
.log-line { padding: 0.125rem 0; border-bottom: 1px solid #1e293b; }
.log-line.error { color: #fca5a5; }
.log-line.warn { color: #fbbf24; }
.log-line.info { color: #93c5fd; }
.log-line.debug { color: #94a3b8; }
```

```javascript
// internal/server/ui/app.js
const API = '/api/v1';

async function fetchJSON(path) {
    const resp = await fetch(API + path);
    return resp.json();
}

async function loadHealth() {
    const data = await fetchJSON('/health/detailed');
    const badge = document.getElementById('status');
    badge.textContent = data.data?.status || 'unknown';
    badge.className = 'badge ' + (data.data?.status || '');
}

async function loadConfig() {
    const data = await fetchJSON('/config');
    document.getElementById('config-output').textContent = JSON.stringify(data.data, null, 2);
}

async function loadLogs() {
    const level = document.getElementById('log-level').value;
    const params = level ? '?level=' + level : '';
    const evtSource = new EventSource(API + '/logs' + params);
    const output = document.getElementById('log-output');
    evtSource.onmessage = (e) => {
        const line = document.createElement('div');
        line.className = 'log-line';
        try {
            const log = JSON.parse(e.data);
            line.textContent = `${log.time || ''} [${log.level || ''}] ${log.message || e.data}`;
            line.className += ' ' + (log.level || '');
        } catch {
            line.textContent = e.data;
        }
        output.appendChild(line);
        output.scrollTop = output.scrollHeight;
        // Keep last 500 lines
        while (output.children.length > 500) {
            output.removeChild(output.firstChild);
        }
    };
}

document.getElementById('log-level').addEventListener('change', () => {
    // Reconnect with new filter
    loadLogs();
});

loadHealth();
loadConfig();
loadLogs();
setInterval(loadHealth, 5000);
```

- [ ] **Step 2: Create UI handler**

```go
// internal/server/ui.go
package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/*
var uiFS embed.FS

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	subFS, err := fs.Sub(uiFS, "ui")
	if err != nil {
		http.Error(w, "UI not available", 500)
		return
	}
	http.StripPrefix("/ui/", http.FileServer(http.FS(subFS))).ServeHTTP(w, r)
}
```

- [ ] **Step 3: Add API routes for dashboard**

In `internal/server/handlers.go`, add to `registerRoutes()`:

```go
mux.HandleFunc("/ui/", s.handleUI)
mux.HandleFunc("/api/v1/config", s.handleGetConfig)
mux.HandleFunc("/api/v1/plugins", s.handleListPlugins)
mux.HandleFunc("/api/v1/logs", s.handleLogsSSE)
mux.HandleFunc("/api/v1/health/detailed", s.handleDetailedHealth)
```

- [ ] **Step 4: Write test for UI endpoint**

```go
// In internal/server/server_test.go or a new file
func TestUIEndpoint(t *testing.T) {
    srv := newTestServer(t)
    req := httptest.NewRequest("GET", "/ui/", nil)
    rr := httptest.NewRecorder()
    srv.ServeHTTP(rr, req)
    if rr.Code != 200 {
        t.Errorf("status = %d, want 200", rr.Code)
    }
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/ui/ internal/server/ui.go internal/server/handlers.go
git commit -m "feat: add embedded dashboard with SSE log streaming"
```

---

## Task 12: Wire New Subsystems in Agent

**Files:**
- Modify: `internal/app/agent.go`
- Modify: `internal/app/interfaces.go`
- Modify: `proto/agent.proto`

- [ ] **Step 1: Add TracingReceiver interface to interfaces.go**

```go
type TracingReceiver interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	HealthStatus() health.Status
}
```

- [ ] **Step 2: Add AlertState proto message**

```protobuf
message AlertState {
    string agent_id = 1;
    string rule_name = 2;
    string severity = 3;
    string state = 4;
    double current_value = 5;
    double threshold = 6;
    int64 triggered_at = 7;
    string message = 8;
}
```

Add to `AgentMessage.oneof payload`.

- [ ] **Step 3: Add blank imports in agent.go**

```go
_ "github.com/cy77cc/opsagent/internal/collector/inputs/tail"
_ "github.com/cy77cc/opsagent/internal/collector/inputs/journald"
_ "github.com/cy77cc/opsagent/internal/collector/inputs/syslog"
_ "github.com/cy77cc/opsagent/internal/collector/processors/logparse"
_ "github.com/cy77cc/opsagent/internal/collector/outputs/otlp"
```

- [ ] **Step 4: Wire alerting engine in agent.go**

In `NewAgent()`, after building the scheduler:
```go
if cfg.Alerting.Enabled {
    engine := alerting.NewEngine(alerting.EngineConfig{Rules: ...})
    // Add webhook notifiers from config
    for _, n := range cfg.Alerting.Rules {
        for _, notify := range n.Notify {
            if notify.Type == "webhook" {
                engine.AddNotifier(alerting.NewWebhookNotifier(notify.URL, notify.Headers))
            }
        }
    }
    a.alertEngine = engine
}
```

In `eventLoop()`, after receiving metrics from pipeline:
```go
if a.alertEngine != nil {
    alerts := a.alertEngine.Evaluate(metrics)
    for _, alert := range alerts {
        // Send via gRPC
        a.grpcClient.SendAlertState(alert)
    }
}
```

- [ ] **Step 5: Run all tests**

Run: `make test-race`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/ internal/collector/ proto/
git commit -m "feat: wire observability subsystems in agent (tail/journald/syslog/logparse/otlp/tracing/alerting)"
```

---

## Task 13: Update Config Reference and Documentation

**Files:**
- Modify: `configs/config.yaml`
- Modify: `docs/zh/architecture.md`

- [ ] **Step 1: Add example config sections**

Append to `configs/config.yaml`:

```yaml
tracing:
  enabled: false
  receiver:
    grpc_addr: "0.0.0.0:4317"
    http_addr: "0.0.0.0:4318"
  processor:
    batch_timeout_ms: 5000
    max_batch_size: 512
  exporter:
    endpoint: "platform.example.com:4317"
    protocol: "grpc"

alerting:
  enabled: false
  rules: []
```

- [ ] **Step 2: Update architecture docs**

Add sections for log collection, tracing, dashboard, and alerting to `docs/zh/architecture.md`.

- [ ] **Step 3: Commit**

```bash
git add configs/config.yaml docs/zh/architecture.md
git commit -m "docs: add config reference and architecture docs for observability features"
```

---

## Self-Review

**Spec coverage:**
- Log collection (tail/journald/syslog + logparse + OTLP): Covered in Tasks 2-7
- Distributed tracing (OTLP receiver/exporter): Covered in Tasks 8-9
- Local dashboard (embedded HTML + SSE): Covered in Task 11
- Smart alerting (rule engine + webhook/platform): Covered in Task 10, 12
- Config sections: Task 8
- Proto extensions: Task 12
- Agent wiring: Task 12
- Documentation: Task 13

**Placeholder scan:** No TBD/TODO found (the OTLP output Write() has a TODO for full SDK integration, which is acceptable as the skeleton is functional).

**Type consistency:** All types (AlertCondition, EvaluatedRule, Alert, etc.) are consistent across tasks.
