package marketplace

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerInstall(t *testing.T) {
	pluginContent := []byte("fake plugin binary content")
	hash := sha256.Sum256(pluginContent)
	checksum := hex.EncodeToString(hash[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(pluginContent)
	}))
	defer srv.Close()

	dir := t.TempDir()
	installer := NewInstaller(dir, srv.Client())

	entry := PluginEntry{
		Name:        "test-plugin",
		Version:     "1.0.0",
		DownloadURL: srv.URL + "/test-plugin.tar.gz",
		Checksum:    checksum,
	}

	if err := installer.Install(entry); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify binary was written.
	binaryPath := filepath.Join(dir, "test-plugin", "test-plugin")
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}
	if string(data) != string(pluginContent) {
		t.Errorf("binary content = %q, want %q", data, pluginContent)
	}

	// Verify metadata file exists.
	metaPath := filepath.Join(dir, "test-plugin", "installed.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		t.Error("installed.yaml not created")
	}
}

func TestInstallerInstall_ChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("wrong content"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	installer := NewInstaller(dir, srv.Client())

	entry := PluginEntry{
		Name:        "bad-plugin",
		Version:     "1.0.0",
		DownloadURL: srv.URL + "/bad-plugin.tar.gz",
		Checksum:    "0000000000000000000000000000000000000000000000000000000000000000",
	}

	err := installer.Install(entry)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

func TestInstallerInstall_DownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dir := t.TempDir()
	installer := NewInstaller(dir, srv.Client())

	entry := PluginEntry{
		Name:        "missing-plugin",
		Version:     "1.0.0",
		DownloadURL: srv.URL + "/missing.tar.gz",
	}

	err := installer.Install(entry)
	if err == nil {
		t.Fatal("expected download error")
	}
}

func TestInstallerInstall_EmptyName(t *testing.T) {
	dir := t.TempDir()
	installer := NewInstaller(dir, nil)

	err := installer.Install(PluginEntry{})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestInstallerInstall_EmptyURL(t *testing.T) {
	dir := t.TempDir()
	installer := NewInstaller(dir, nil)

	err := installer.Install(PluginEntry{Name: "test"})
	if err == nil {
		t.Fatal("expected error for empty download URL")
	}
}

func TestInstallerRemove(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "my-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}

	installer := NewInstaller(dir, nil)

	if err := installer.Remove("my-plugin"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Error("plugin directory should have been removed")
	}
}

func TestInstallerRemove_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	installer := NewInstaller(dir, nil)

	err := installer.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestInstallerRemove_EmptyName(t *testing.T) {
	dir := t.TempDir()
	installer := NewInstaller(dir, nil)

	err := installer.Remove("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestInstallerList(t *testing.T) {
	dir := t.TempDir()

	// Create two fake installed plugins.
	for _, name := range []string{"plugin-a", "plugin-b"} {
		pluginDir := filepath.Join(dir, name)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatal(err)
		}
		meta := `name: ` + name + `
version: "1.0.0"
installed_at: "2024-01-01T00:00:00Z"
`
		if err := os.WriteFile(filepath.Join(pluginDir, "installed.yaml"), []byte(meta), 0644); err != nil {
			t.Fatal(err)
		}
	}

	installer := NewInstaller(dir, nil)

	plugins, err := installer.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("List returned %d plugins, want 2", len(plugins))
	}
}

func TestInstallerList_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	installer := NewInstaller(dir, nil)

	plugins, err := installer.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(plugins) != 0 {
		t.Errorf("List returned %d plugins, want 0", len(plugins))
	}
}

func TestInstallerList_NonexistentDir(t *testing.T) {
	installer := NewInstaller("/nonexistent/path", nil)

	plugins, err := installer.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if plugins != nil {
		t.Errorf("List returned %v, want nil", plugins)
	}
}
