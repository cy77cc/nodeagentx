package journald

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"

	"github.com/cy77cc/opsagent/internal/collector"
)

// ---------------------------------------------------------------------------
// mock journal
// ---------------------------------------------------------------------------

type mockJournal struct {
	entries   []*sdjournal.JournalEntry
	cursor    int
	closed    bool
	waitCalls int

	// Error injection points
	nextErr    error
	getErr     error
	cursorErr  error
	addMatchErr error
	disjErr    error
	seekTailErr error
	seekCurErr error

	// Recorded arguments for assertions
	addedMatches []string
	seekedCursor string
}

func (m *mockJournal) Close() error { m.closed = true; return nil }

func (m *mockJournal) AddMatch(match string) error {
	m.addedMatches = append(m.addedMatches, match)
	return m.addMatchErr
}

func (m *mockJournal) AddDisjunction() error { return m.disjErr }

func (m *mockJournal) SeekTail() error { return m.seekTailErr }

func (m *mockJournal) SeekCursor(cursor string) error {
	m.seekedCursor = cursor
	return m.seekCurErr
}

func (m *mockJournal) GetCursor() (string, error) {
	if m.cursorErr != nil {
		return "", m.cursorErr
	}
	if m.cursor == 0 {
		return "", nil
	}
	return fmt.Sprintf("cursor-%d", m.cursor), nil
}

func (m *mockJournal) Wait(timeout time.Duration) int {
	m.waitCalls++
	return 0
}

func (m *mockJournal) Next() (uint64, error) {
	if m.nextErr != nil {
		return 0, m.nextErr
	}
	if m.cursor >= len(m.entries) {
		return 0, nil
	}
	m.cursor++
	return uint64(m.cursor), nil
}

func (m *mockJournal) GetEntry() (*sdjournal.JournalEntry, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.cursor == 0 || m.cursor > len(m.entries) {
		return nil, errors.New("no entry at current position")
	}
	return m.entries[m.cursor-1], nil
}

func makeEntry(msg string, unit string, prio string, ts uint64) *sdjournal.JournalEntry {
	fields := map[string]string{
		"MESSAGE":  msg,
		"PRIORITY": prio,
	}
	if unit != "" {
		fields["_SYSTEMD_UNIT"] = unit
	}
	return &sdjournal.JournalEntry{
		Fields:            fields,
		RealtimeTimestamp: ts,
	}
}

func newMockFactory(journal *mockJournal) func() (journalReader, error) {
	return func() (journalReader, error) {
		return journal, nil
	}
}

// ---------------------------------------------------------------------------
// Init tests
// ---------------------------------------------------------------------------

func TestJournaldInputInit(t *testing.T) {
	ji := &JournaldInput{}
	cfg := map[string]interface{}{
		"units":               []interface{}{"nginx", "sshd"},
		"priority":            "info",
		"cursor_persist_path": "/tmp/journal.cursor",
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
	if ji.CursorPersistPath != "/tmp/journal.cursor" {
		t.Errorf("CursorPersistPath = %q, want /tmp/journal.cursor", ji.CursorPersistPath)
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
		{"warn", 4, false},
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

func TestJournaldInputInitInvalidUnits(t *testing.T) {
	ji := &JournaldInput{}
	cfg := map[string]interface{}{
		"units": "not-a-list",
	}
	if err := ji.Init(cfg); err == nil {
		t.Error("Init should fail with invalid units type")
	}
}

func TestJournaldInputInitInvalidPriority(t *testing.T) {
	ji := &JournaldInput{}
	cfg := map[string]interface{}{
		"priority": 123,
	}
	if err := ji.Init(cfg); err == nil {
		t.Error("Init should fail with invalid priority type")
	}
}

func TestJournaldInputInitInvalidCursorPath(t *testing.T) {
	ji := &JournaldInput{}
	cfg := map[string]interface{}{
		"cursor_persist_path": 123,
	}
	if err := ji.Init(cfg); err == nil {
		t.Error("Init should fail with invalid cursor_persist_path type")
	}
}

func TestJournaldInputInitInvalidUnitEntry(t *testing.T) {
	ji := &JournaldInput{}
	cfg := map[string]interface{}{
		"units": []interface{}{123},
	}
	if err := ji.Init(cfg); err == nil {
		t.Error("Init should fail with non-string unit entry")
	}
}

// ---------------------------------------------------------------------------
// Gather tests
// ---------------------------------------------------------------------------

func TestGatherWithEntries(t *testing.T) {
	entry := makeEntry("hello world", "nginx.service", "6", 1700000000000000)
	mock := &mockJournal{entries: []*sdjournal.JournalEntry{entry}}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Units:       []string{"nginx"},
		Priority:    "info",
		priorityVal: 6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	m := metrics[0]
	if m.Name() != "journald" {
		t.Errorf("name = %q, want journald", m.Name())
	}
	fields := m.Fields()
	if fields["message"] != "hello world" {
		t.Errorf("message = %v, want 'hello world'", fields["message"])
	}
	if fields["priority"] != "6" {
		t.Errorf("priority = %v, want '6'", fields["priority"])
	}
	// Verify "timestamp" field is NOT in fields (uses AddGaugeWithTimestamp instead)
	if _, ok := fields["timestamp"]; ok {
		t.Error("fields should not contain 'timestamp' key")
	}
	// Verify "unit" is NOT in fields (only in tags)
	if _, ok := fields["unit"]; ok {
		t.Error("fields should not contain 'unit' key (it belongs in tags)")
	}
	// Verify unit is in tags
	tags := m.Tags()
	if tags["unit"] != "nginx.service" {
		t.Errorf("tags[unit] = %q, want nginx.service", tags["unit"])
	}
	// Verify timestamp is from the entry, not time.Now()
	ts := m.Timestamp()
	expected := time.UnixMicro(int64(1700000000000000))
	if !ts.Equal(expected) {
		t.Errorf("timestamp = %v, want %v", ts, expected)
	}
}

func TestGatherRespectsContextCancellation(t *testing.T) {
	// Create enough entries that the loop would continue
	entries := make([]*sdjournal.JournalEntry, 100)
	for i := range entries {
		entries[i] = makeEntry("msg", "svc.service", "6", uint64(1700000000+i))
	}
	mock := &mockJournal{entries: entries}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Priority:    "info",
		priorityVal: 6,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	acc := collector.NewAccumulator(100)
	err := ji.Gather(ctx, acc)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestGatherRespects1000EntryLimit(t *testing.T) {
	entries := make([]*sdjournal.JournalEntry, 1500)
	for i := range entries {
		entries[i] = makeEntry("msg", "svc.service", "6", uint64(1700000000+i))
	}
	mock := &mockJournal{entries: entries}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Priority:    "info",
		priorityVal: 6,
	}

	acc := collector.NewAccumulator(2000)
	err := ji.Gather(context.Background(), acc)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) != 1000 {
		t.Errorf("expected 1000 metrics (limit), got %d", len(metrics))
	}
}

func TestGatherHandlesJournalOpenError(t *testing.T) {
	ji := &JournaldInput{
		newJournal: func() (journalReader, error) {
			return nil, errors.New("open failed")
		},
		Priority:    "info",
		priorityVal: 6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error from journal open failure")
	}
}

func TestGatherHandlesNextError(t *testing.T) {
	mock := &mockJournal{
		entries: []*sdjournal.JournalEntry{makeEntry("msg", "svc.service", "6", 1700000000)},
		nextErr: errors.New("next failed"),
	}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Priority:    "info",
		priorityVal: 6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error from Next failure")
	}
}

func TestGatherHandlesGetEntryError(t *testing.T) {
	// Create enough entries so GetEntry can fail maxGetEntryErrors times
	entries := make([]*sdjournal.JournalEntry, maxGetEntryErrors+5)
	for i := range entries {
		entries[i] = makeEntry("msg", "svc.service", "6", 1700000000+uint64(i))
	}
	mock := &mockJournal{
		entries: entries,
		getErr:  errors.New("get entry failed"),
	}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Priority:    "info",
		priorityVal: 6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err == nil {
		t.Fatal("expected error after too many GetEntry failures")
	}
}

func TestGatherPriorityFilterAlwaysApplied(t *testing.T) {
	// This tests CRITICAL 2 fix: priority filter is always applied, even for emerg (0)
	mock := &mockJournal{entries: nil}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Priority:    "emerg",
		priorityVal: 0,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	// Verify that AddDisjunction was called (priority filter applied)
	// Even with priority=emerg (0), the disjunction and PRIORITY=0 match should be added
	hasPriorityMatch := false
	for _, m := range mock.addedMatches {
		if m == "PRIORITY=0" {
			hasPriorityMatch = true
		}
	}
	if !hasPriorityMatch {
		t.Error("priority filter was not applied for 'emerg' (value 0)")
	}
}

func TestGatherUnitSuffixSmart(t *testing.T) {
	mock := &mockJournal{entries: nil}

	ji := &JournaldInput{
		newJournal:  newMockFactory(mock),
		Units:       []string{"nginx", "docker.socket", "tmp.mount"},
		Priority:    "info",
		priorityVal: 6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	expected := []string{
		"_SYSTEMD_UNIT=nginx.service",
		"_SYSTEMD_UNIT=docker.socket",
		"_SYSTEMD_UNIT=tmp.mount",
	}
	if len(mock.addedMatches) < len(expected) {
		t.Fatalf("expected at least %d matches, got %d", len(expected), len(mock.addedMatches))
	}
	for i, want := range expected {
		if mock.addedMatches[i] != want {
			t.Errorf("match[%d] = %q, want %q", i, mock.addedMatches[i], want)
		}
	}
}

// ---------------------------------------------------------------------------
// Cursor persistence tests
// ---------------------------------------------------------------------------

func TestGatherCursorSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	entry := makeEntry("msg", "svc.service", "6", 1700000000)
	mock := &mockJournal{entries: []*sdjournal.JournalEntry{entry}}

	ji := &JournaldInput{
		newJournal:       newMockFactory(mock),
		CursorPersistPath: dir,
		Priority:         "info",
		priorityVal:      6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err != nil {
		t.Fatalf("first Gather: %v", err)
	}

	// Verify cursor file was created
	cursorPath := filepath.Join(dir, "journald.cursor")
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read cursor file: %v", err)
	}

	var cf cursorFile
	if err := json.Unmarshal(data, &cf); err != nil {
		t.Fatalf("unmarshal cursor: %v", err)
	}
	if cf.Cursor == "" {
		t.Error("saved cursor should not be empty")
	}

	// Second gather: should restore cursor and pass it to SeekCursor
	mock2 := &mockJournal{entries: nil}
	ji2 := &JournaldInput{
		newJournal:       newMockFactory(mock2),
		CursorPersistPath: dir,
		Priority:         "info",
		priorityVal:      6,
	}

	acc2 := collector.NewAccumulator(100)
	err = ji2.Gather(context.Background(), acc2)
	if err != nil {
		t.Fatalf("second Gather: %v", err)
	}

	// Verify cursor was restored
	if mock2.seekedCursor != cf.Cursor {
		t.Errorf("SeekCursor called with %q, want %q", mock2.seekedCursor, cf.Cursor)
	}
}

func TestLoadCursorMissingFile(t *testing.T) {
	dir := t.TempDir()

	ji := &JournaldInput{CursorPersistPath: dir}
	cursor, err := ji.loadCursor()
	if err != nil {
		t.Fatalf("loadCursor on missing file should return nil error, got: %v", err)
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
}

func TestLoadCursorEmptyPath(t *testing.T) {
	ji := &JournaldInput{}
	cursor, err := ji.loadCursor()
	if err != nil {
		t.Fatalf("loadCursor with empty path: %v", err)
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
}

func TestSaveCursorEmptyPath(t *testing.T) {
	ji := &JournaldInput{}
	// Should be a no-op, not an error
	if err := ji.saveCursor("test-cursor"); err != nil {
		t.Fatalf("saveCursor with empty path should not error: %v", err)
	}
}

func TestLoadCursorInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cursorPath := filepath.Join(dir, "journald.cursor")
	if err := os.WriteFile(cursorPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	ji := &JournaldInput{CursorPersistPath: dir}
	_, err := ji.loadCursor()
	if err == nil {
		t.Error("loadCursor with invalid JSON should return error")
	}
}

func TestGatherCursorStaleFallback(t *testing.T) {
	dir := t.TempDir()

	// Write a cursor file
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cf := cursorFile{Cursor: "stale-cursor"}
	data, _ := json.Marshal(cf)
	if err := os.WriteFile(filepath.Join(dir, "journald.cursor"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Make SeekCursor return an error (stale cursor)
	mock := &mockJournal{
		entries:    nil,
		seekCurErr: errors.New("cursor not found"),
	}

	ji := &JournaldInput{
		newJournal:       newMockFactory(mock),
		CursorPersistPath: dir,
		Priority:         "info",
		priorityVal:      6,
	}

	acc := collector.NewAccumulator(100)
	err := ji.Gather(context.Background(), acc)
	if err != nil {
		t.Fatalf("Gather should handle stale cursor gracefully: %v", err)
	}
}

// ---------------------------------------------------------------------------
// unitSuffix tests
// ---------------------------------------------------------------------------

func TestUnitSuffix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nginx", "nginx.service"},
		{"sshd", "sshd.service"},
		{"docker.socket", "docker.socket"},
		{"tmp.mount", "tmp.mount"},
		{"dbus.service", "dbus.service"},
	}
	for _, tt := range tests {
		got := unitSuffix(tt.input)
		if got != tt.want {
			t.Errorf("unitSuffix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
