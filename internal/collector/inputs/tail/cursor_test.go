package tail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCursorSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cursorPath := filepath.Join(dir, "cursor.json")

	original := &Cursor{
		Path:   "/var/log/syslog",
		Offset: 12345,
		Inode:  67890,
	}

	if err := original.Save(cursorPath); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := LoadCursor(cursorPath)
	if err != nil {
		t.Fatalf("LoadCursor() failed: %v", err)
	}

	if loaded.Path != original.Path {
		t.Errorf("Path = %q, want %q", loaded.Path, original.Path)
	}
	if loaded.Offset != original.Offset {
		t.Errorf("Offset = %d, want %d", loaded.Offset, original.Offset)
	}
	if loaded.Inode != original.Inode {
		t.Errorf("Inode = %d, want %d", loaded.Inode, original.Inode)
	}
}

func TestLoadCursorMissingFile(t *testing.T) {
	_, err := LoadCursor("/nonexistent/path/cursor.json")
	if err == nil {
		t.Fatal("LoadCursor() on missing file should return error, got nil")
	}
}

func TestSaveInvalidPath(t *testing.T) {
	c := &Cursor{Path: "/var/log/app.log", Offset: 100, Inode: 200}
	// /dev/null is not a directory, so writing a file inside it should fail.
	err := c.Save("/dev/null/cursor.json")
	if err == nil {
		t.Fatal("Save() to invalid path should return error, got nil")
	}
}

func TestLoadCursorInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cursorPath := filepath.Join(dir, "cursor.json")
	if err := os.WriteFile(cursorPath, []byte("not valid json"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	_, err := LoadCursor(cursorPath)
	if err == nil {
		t.Fatal("LoadCursor() with invalid JSON should return error, got nil")
	}
}

func TestCursorSaveCreatesDir(t *testing.T) {
	dir := t.TempDir()
	cursorPath := filepath.Join(dir, "deep", "nested", "dir", "cursor.json")

	c := &Cursor{
		Path:   "/var/log/app.log",
		Offset: 100,
		Inode:  200,
	}

	if err := c.Save(cursorPath); err != nil {
		t.Fatalf("Save() should create parent directories, got error: %v", err)
	}

	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("Failed to read saved cursor file: %v", err)
	}

	var loaded Cursor
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Failed to unmarshal saved cursor: %v", err)
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
