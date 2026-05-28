package marketplace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// InstalledPlugin tracks metadata for an installed plugin.
type InstalledPlugin struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	InstalledAt string `yaml:"installed_at"`
}

// Installer handles downloading, extracting, and managing plugin installations.
type Installer struct {
	pluginsDir string
	client     *http.Client
}

// NewInstaller creates a new Installer that stores plugins under pluginsDir.
func NewInstaller(pluginsDir string, client *http.Client) *Installer {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &Installer{
		pluginsDir: pluginsDir,
		client:     client,
	}
}

// Install downloads and installs a plugin from the given entry.
// It verifies the checksum and writes a metadata file.
func (i *Installer) Install(entry PluginEntry) error {
	if entry.Name == "" {
		return fmt.Errorf("plugin name is required")
	}
	if entry.DownloadURL == "" {
		return fmt.Errorf("plugin download URL is required")
	}

	pluginDir := filepath.Join(i.pluginsDir, entry.Name)

	// Create plugin directory.
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("create plugin directory: %w", err)
	}

	// Download the plugin archive.
	resp, err := i.client.Get(entry.DownloadURL)
	if err != nil {
		return fmt.Errorf("download plugin %s: %w", entry.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download plugin %s: unexpected status %d", entry.Name, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read plugin download: %w", err)
	}

	// Verify checksum if provided.
	if entry.Checksum != "" {
		hash := sha256.Sum256(data)
		actual := hex.EncodeToString(hash[:])
		if actual != entry.Checksum {
			return fmt.Errorf("checksum mismatch for plugin %s: expected %s, got %s", entry.Name, entry.Checksum, actual)
		}
	}

	// Write plugin binary/content.
	binaryPath := filepath.Join(pluginDir, entry.Name)
	if err := os.WriteFile(binaryPath, data, 0755); err != nil {
		return fmt.Errorf("write plugin binary: %w", err)
	}

	// Write metadata.
	meta := InstalledPlugin{
		Name:        entry.Name,
		Version:     entry.Version,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	metaPath := filepath.Join(pluginDir, "installed.yaml")
	metaData, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal plugin metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		return fmt.Errorf("write plugin metadata: %w", err)
	}

	return nil
}

// Remove uninstalls a plugin by name, deleting its directory.
func (i *Installer) Remove(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name is required")
	}

	pluginDir := filepath.Join(i.pluginsDir, name)
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return fmt.Errorf("plugin %q is not installed", name)
	}

	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("remove plugin %s: %w", name, err)
	}
	return nil
}

// List returns all installed plugins by reading metadata files from the plugins directory.
func (i *Installer) List() ([]InstalledPlugin, error) {
	entries, err := os.ReadDir(i.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins directory: %w", err)
	}

	var plugins []InstalledPlugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(i.pluginsDir, entry.Name(), "installed.yaml")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue // skip plugins without metadata
		}
		var meta InstalledPlugin
		if err := yaml.Unmarshal(data, &meta); err != nil {
			continue // skip invalid metadata
		}
		plugins = append(plugins, meta)
	}
	return plugins, nil
}
