package marketplace

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// PluginEntry represents a single plugin in the registry index.
type PluginEntry struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	Homepage    string   `json:"homepage"`
	Tags        []string `json:"tags"`
	DownloadURL string   `json:"download_url"`
	Checksum    string   `json:"checksum"`
}

// RegistryIndex is the full index of available plugins.
type RegistryIndex struct {
	Version   string        `json:"version"`
	UpdatedAt time.Time     `json:"updated_at"`
	Plugins   []PluginEntry `json:"plugins"`
}

// Registry provides access to the plugin registry.
type Registry struct {
	indexURL string
	client   *http.Client

	mu    sync.RWMutex
	index *RegistryIndex
}

// NewRegistry creates a new Registry that fetches its index from indexURL.
func NewRegistry(indexURL string, client *http.Client) *Registry {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Registry{
		indexURL: indexURL,
		client:   client,
	}
}

// fetchIndex retrieves and parses the plugin index from the remote URL.
// The caller must hold r.mu (write lock).
func (r *Registry) fetchIndex() error {
	resp, err := r.client.Get(r.indexURL)
	if err != nil {
		return fmt.Errorf("fetch registry index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch registry index: unexpected status %d", resp.StatusCode)
	}

	var idx RegistryIndex
	if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return fmt.Errorf("decode registry index: %w", err)
	}
	r.index = &idx
	return nil
}

// Search returns all plugins whose name or description contains the query (case-insensitive).
// It fetches the index on first call and caches it.
func (r *Registry) Search(query string) ([]PluginEntry, error) {
	r.mu.Lock()
	if r.index == nil {
		if err := r.fetchIndex(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
	}
	r.mu.Unlock()

	r.mu.RLock()
	defer r.mu.RUnlock()

	q := strings.ToLower(query)
	var results []PluginEntry
	for _, p := range r.index.Plugins {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Description), q) {
			results = append(results, p)
		}
	}
	return results, nil
}

// Get returns a single plugin entry by exact name. Returns an error if not found.
func (r *Registry) Get(name string) (*PluginEntry, error) {
	r.mu.Lock()
	if r.index == nil {
		if err := r.fetchIndex(); err != nil {
			r.mu.Unlock()
			return nil, err
		}
	}
	r.mu.Unlock()

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.index.Plugins {
		if p.Name == name {
			p := p // copy
			return &p, nil
		}
	}
	return nil, fmt.Errorf("plugin %q not found", name)
}
