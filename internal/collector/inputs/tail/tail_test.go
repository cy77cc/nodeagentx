package tail

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestTailInputInit(t *testing.T) {
	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files":               []interface{}{"/var/log/syslog", "/var/log/auth.log"},
		"watch_method":        "inotify",
		"from_beginning":      true,
		"cursor_persist_path": "/tmp/cursor.json",
		"max_line_bytes":      4096,
	}
	if err := ti.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if len(ti.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(ti.Files))
	}
	if ti.Files[0] != "/var/log/syslog" {
		t.Errorf("expected first file /var/log/syslog, got %s", ti.Files[0])
	}
	if ti.Files[1] != "/var/log/auth.log" {
		t.Errorf("expected second file /var/log/auth.log, got %s", ti.Files[1])
	}
	if ti.WatchMethod != "inotify" {
		t.Errorf("expected watch_method=inotify, got %s", ti.WatchMethod)
	}
	if !ti.FromBeginning {
		t.Errorf("expected from_beginning=true")
	}
	if ti.CursorPersistPath != "/tmp/cursor.json" {
		t.Errorf("expected cursor_persist_path=/tmp/cursor.json, got %s", ti.CursorPersistPath)
	}
	if ti.MaxLineBytes != 4096 {
		t.Errorf("expected max_line_bytes=4096, got %d", ti.MaxLineBytes)
	}
}

func TestTailInputInitDefaults(t *testing.T) {
	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files": []interface{}{"/tmp/test.log"},
	}
	if err := ti.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if ti.WatchMethod != "poll" {
		t.Errorf("expected default watch_method=poll, got %s", ti.WatchMethod)
	}
	if ti.MaxLineBytes != 65536 {
		t.Errorf("expected default max_line_bytes=65536, got %d", ti.MaxLineBytes)
	}
	if ti.FromBeginning {
		t.Errorf("expected default from_beginning=false")
	}
}

func TestTailInputGather(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test log: %v", err)
	}

	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files":          []interface{}{logFile},
		"from_beginning": true,
	}
	if err := ti.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	acc := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc); err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) != 3 {
		t.Fatalf("expected 3 metrics, got %d", len(metrics))
	}

	expectedMessages := []string{"line one", "line two", "line three"}
	for i, m := range metrics {
		if m.Name() != "tail" {
			t.Errorf("metric %d: expected name=tail, got %s", i, m.Name())
		}
		tags := m.Tags()
		if tags["file"] != logFile {
			t.Errorf("metric %d: expected file tag=%s, got %s", i, logFile, tags["file"])
		}
		fields := m.Fields()
		if fields["message"] != expectedMessages[i] {
			t.Errorf("metric %d: expected message=%q, got %v", i, expectedMessages[i], fields["message"])
		}
	}
}

func TestTailInputGatherAppend(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "append.log")

	// Write initial content
	initial := "first line\nsecond line\n"
	if err := os.WriteFile(logFile, []byte(initial), 0o644); err != nil {
		t.Fatalf("failed to write test log: %v", err)
	}

	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files":          []interface{}{logFile},
		"from_beginning": true,
	}
	if err := ti.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	acc := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc); err != nil {
		t.Fatalf("first Gather failed: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics from first gather, got %d", len(metrics))
	}

	// Append more content
	extra := "third line\nfourth line\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open test log for append: %v", err)
	}
	if _, err := f.WriteString(extra); err != nil {
		f.Close()
		t.Fatalf("failed to append to test log: %v", err)
	}
	f.Close()

	acc2 := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc2); err != nil {
		t.Fatalf("second Gather failed: %v", err)
	}

	metrics2 := acc2.Collect()
	if len(metrics2) != 2 {
		t.Fatalf("expected 2 metrics from second gather (only new lines), got %d", len(metrics2))
	}

	expectedMessages := []string{"third line", "fourth line"}
	for i, m := range metrics2 {
		fields := m.Fields()
		if fields["message"] != expectedMessages[i] {
			t.Errorf("metric %d: expected message=%q, got %v", i, expectedMessages[i], fields["message"])
		}
	}
}

func TestTailInputSampleConfig(t *testing.T) {
	ti := &TailInput{}
	if ti.SampleConfig() == "" {
		t.Errorf("expected non-empty sample config")
	}
}
