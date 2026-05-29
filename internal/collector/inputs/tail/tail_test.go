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
		"watch_method":        "poll",
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
	if ti.WatchMethod != "poll" {
		t.Errorf("expected watch_method=poll, got %s", ti.WatchMethod)
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

func TestTailInputGatherFromEnd(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "end.log")

	content := "line1\nline2\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test log: %v", err)
	}

	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files":          []interface{}{logFile},
		"from_beginning": false,
	}
	if err := ti.Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	acc := collector.NewAccumulator(100)
	if err := ti.Gather(context.Background(), acc); err != nil {
		t.Fatalf("first Gather failed: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) != 0 {
		t.Fatalf("expected 0 metrics from first gather (seeks to end), got %d", len(metrics))
	}

	// Append a new line
	extra := "line3\n"
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
	if len(metrics2) != 1 {
		t.Fatalf("expected 1 metric from second gather, got %d", len(metrics2))
	}

	fields := metrics2[0].Fields()
	if fields["message"] != "line3" {
		t.Errorf("expected message=%q, got %v", "line3", fields["message"])
	}
}

func TestTailInputInitInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]interface{}
	}{
		{
			name: "files not a list",
			cfg: map[string]interface{}{
				"files": "not-a-list",
			},
		},
		{
			name: "file entry not a string",
			cfg: map[string]interface{}{
				"files": []interface{}{123},
			},
		},
		{
			name: "watch_method not a string",
			cfg: map[string]interface{}{
				"watch_method": 123,
			},
		},
		{
			name: "watch_method unsupported value",
			cfg: map[string]interface{}{
				"watch_method": "inotify",
			},
		},
		{
			name: "from_beginning not a bool",
			cfg: map[string]interface{}{
				"from_beginning": "yes",
			},
		},
		{
			name: "cursor_persist_path not a string",
			cfg: map[string]interface{}{
				"cursor_persist_path": 123,
			},
		},
		{
			name: "max_line_bytes not a number",
			cfg: map[string]interface{}{
				"max_line_bytes": "big",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := &TailInput{}
			if err := ti.Init(tt.cfg); err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}

func TestTailInputWatchMethodValidation(t *testing.T) {
	ti := &TailInput{}
	cfg := map[string]interface{}{
		"watch_method": "inotify",
	}
	err := ti.Init(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported watch_method, got nil")
	}
	expected := `unsupported watch_method "inotify"`
	if got := err.Error(); !contains(got, expected) {
		t.Errorf("expected error containing %q, got %q", expected, got)
	}
}

func TestTailInputGatherMetricsAreGauges(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "gauge.log")

	if err := os.WriteFile(logFile, []byte("hello\n"), 0o644); err != nil {
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
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	if metrics[0].Type() != collector.Gauge {
		t.Errorf("expected metric type Gauge, got %v", metrics[0].Type())
	}
}

func TestTailInputCursorPersistence(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, "cursors")
	logFile := filepath.Join(dir, "persist.log")

	initial := "line1\nline2\n"
	if err := os.WriteFile(logFile, []byte(initial), 0o644); err != nil {
		t.Fatalf("failed to write test log: %v", err)
	}

	ti := &TailInput{}
	cfg := map[string]interface{}{
		"files":               []interface{}{logFile},
		"from_beginning":      true,
		"cursor_persist_path": cursorDir,
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
		t.Fatalf("expected 2 metrics, got %d", len(metrics))
	}

	// Verify cursor file was created
	cursorPath := cursorFilename(cursorDir, logFile)
	if _, err := os.Stat(cursorPath); os.IsNotExist(err) {
		t.Fatalf("expected cursor file at %s, not found", cursorPath)
	}

	// Load the cursor and verify it matches
	c, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("failed to load cursor: %v", err)
	}
	if c.Path != logFile {
		t.Errorf("expected cursor path=%s, got %s", logFile, c.Path)
	}
	if c.Offset <= 0 {
		t.Errorf("expected positive offset, got %d", c.Offset)
	}

	// Create a new TailInput that loads the persisted cursor
	ti2 := &TailInput{}
	if err := ti2.Init(cfg); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}

	// The loaded cursor should have populated offsets
	ti2.mu.Lock()
	loadedOffset := ti2.offsets[logFile]
	ti2.mu.Unlock()
	if loadedOffset != c.Offset {
		t.Errorf("expected loaded offset=%d, got %d", c.Offset, loadedOffset)
	}

	// Append more content and verify only new lines are read
	extra := "line3\n"
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open for append: %v", err)
	}
	if _, err := f.WriteString(extra); err != nil {
		f.Close()
		t.Fatalf("failed to append: %v", err)
	}
	f.Close()

	acc2 := collector.NewAccumulator(100)
	if err := ti2.Gather(context.Background(), acc2); err != nil {
		t.Fatalf("second Gather failed: %v", err)
	}

	metrics2 := acc2.Collect()
	if len(metrics2) != 1 {
		t.Fatalf("expected 1 metric from second gather, got %d", len(metrics2))
	}
	if metrics2[0].Fields()["message"] != "line3" {
		t.Errorf("expected message=%q, got %v", "line3", metrics2[0].Fields()["message"])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
