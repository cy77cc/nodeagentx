package discovery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMetadataURL  = "http://169.254.169.254/latest/meta-data/"
	defaultClientTimeout = 2 * time.Second
	maxResponseBytes     = 1024
)

// MetadataLayer discovers cloud instance metadata from the EC2 metadata service.
type MetadataLayer struct {
	metadataURL string
	client      *http.Client
}

// NewMetadataLayer creates a MetadataLayer with default settings.
func NewMetadataLayer() *MetadataLayer {
	return &MetadataLayer{
		metadataURL: defaultMetadataURL,
		client:      &http.Client{Timeout: defaultClientTimeout},
	}
}

// Name returns "metadata".
func (m *MetadataLayer) Name() string {
	return "metadata"
}

// Discover fetches instance metadata and returns a Service representing the cloud instance.
func (m *MetadataLayer) Discover(ctx context.Context) ([]Service, error) {
	client := m.client
	if client == nil {
		client = &http.Client{Timeout: defaultClientTimeout}
	}

	baseURL := m.metadataURL
	if baseURL == "" {
		baseURL = defaultMetadataURL
	}

	// Map of metadata paths to result keys.
	paths := map[string]string{
		"instance-id":      "instance_id",
		"instance-type":    "instance_type",
		"placement/region": "region",
		"local-ipv4":       "local_ip",
	}

	values := make(map[string]string)
	for path, key := range paths {
		val, err := fetch(ctx, client, baseURL+path)
		if err != nil {
			// Non-fatal: some metadata keys may not be available.
			continue
		}
		values[key] = val
	}

	// If instance_id is empty, no metadata available.
	if strings.TrimSpace(values["instance_id"]) == "" {
		return nil, nil
	}

	svc := Service{
		Name: values["instance_id"],
		Type: "cloud_metadata",
		Labels: map[string]string{
			"cloud":  "aws",
			"region": values["region"],
		},
		Metadata: map[string]any{
			"instance_id":   values["instance_id"],
			"instance_type": values["instance_type"],
			"local_ip":      values["local_ip"],
		},
	}

	return []Service{svc}, nil
}

// fetch performs a GET request and returns the response body as a trimmed string.
// It reads up to 1024 bytes and requires a 200 status code.
func fetch(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading response from %s: %w", url, err)
	}

	return strings.TrimSpace(string(body)), nil
}
