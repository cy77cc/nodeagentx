package journald

import (
	"testing"
)

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
