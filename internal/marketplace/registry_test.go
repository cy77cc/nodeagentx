package marketplace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistrySearch(t *testing.T) {
	index := RegistryIndex{
		Version: "1",
		Plugins: []PluginEntry{
			{Name: "audit-plugin", Version: "1.0.0", Description: "Security audit tool", Tags: []string{"security"}},
			{Name: "log-plugin", Version: "2.0.0", Description: "Log aggregation plugin", Tags: []string{"logging"}},
			{Name: "monitor-plugin", Version: "1.5.0", Description: "System monitoring and alerting", Tags: []string{"monitoring"}},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(index)
	}))
	defer srv.Close()

	r := NewRegistry(srv.URL, srv.Client())

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantFirst string
	}{
		{"match by name", "audit", 1, "audit-plugin"},
		{"match by description", "monitoring", 1, "monitor-plugin"},
		{"match multiple", "plugin", 3, ""},
		{"no match", "nonexistent", 0, ""},
		{"case insensitive", "Audit", 1, "audit-plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := r.Search(tt.query)
			if err != nil {
				t.Fatalf("Search(%q): %v", tt.query, err)
			}
			if len(results) != tt.wantCount {
				t.Fatalf("Search(%q) returned %d results, want %d", tt.query, len(results), tt.wantCount)
			}
			if tt.wantFirst != "" && results[0].Name != tt.wantFirst {
				t.Errorf("Search(%q) first result = %q, want %q", tt.query, results[0].Name, tt.wantFirst)
			}
		})
	}
}

func TestRegistrySearch_FetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	r := NewRegistry(srv.URL, srv.Client())
	_, err := r.Search("anything")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRegistryGet(t *testing.T) {
	index := RegistryIndex{
		Version: "1",
		Plugins: []PluginEntry{
			{Name: "audit-plugin", Version: "1.0.0", Description: "Security audit tool"},
			{Name: "log-plugin", Version: "2.0.0", Description: "Log aggregation"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(index)
	}))
	defer srv.Close()

	r := NewRegistry(srv.URL, srv.Client())

	entry, err := r.Get("audit-plugin")
	if err != nil {
		t.Fatalf("Get(audit-plugin): %v", err)
	}
	if entry.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", entry.Version, "1.0.0")
	}

	_, err = r.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

func TestRegistrySearch_NetworkError(t *testing.T) {
	// Use a URL that will fail to connect.
	r := NewRegistry("http://127.0.0.1:1", nil)
	_, err := r.Search("anything")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestRegistrySearch_CachesIndex(t *testing.T) {
	fetchCount := 0
	index := RegistryIndex{
		Version: "1",
		Plugins: []PluginEntry{
			{Name: "test-plugin", Version: "1.0.0", Description: "Test"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(index)
	}))
	defer srv.Close()

	r := NewRegistry(srv.URL, srv.Client())

	// First call should fetch.
	_, err := r.Search("test")
	if err != nil {
		t.Fatalf("first search: %v", err)
	}
	// Second call should use cache.
	_, err = r.Search("test")
	if err != nil {
		t.Fatalf("second search: %v", err)
	}

	if fetchCount != 1 {
		t.Errorf("fetchCount = %d, want 1 (index should be cached)", fetchCount)
	}
}
