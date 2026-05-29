package cloudmetadata

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func init() {
	collector.RegisterInput("cloud_metadata", func() collector.Input {
		return &MetadataInput{}
	})
}

const (
	defaultMetadataURL = "http://169.254.169.254/latest/meta-data/"
	defaultTimeout     = 2
)

// MetadataInput fetches cloud instance metadata from the metadata service.
type MetadataInput struct {
	metadataURL string
	client      *http.Client
	Timeout     int
}

// Init parses the configuration map and sets defaults.
func (m *MetadataInput) Init(cfg map[string]interface{}) error {
	m.metadataURL = defaultMetadataURL
	m.Timeout = defaultTimeout

	if url, ok := cfg["metadata_url"].(string); ok && url != "" {
		m.metadataURL = url
	}
	if timeout, ok := cfg["timeout"].(int); ok && timeout > 0 {
		m.Timeout = timeout
	} else if timeoutF, ok := cfg["timeout"].(float64); ok && timeoutF > 0 {
		m.Timeout = int(timeoutF)
	}

	m.client = &http.Client{
		Timeout: time.Duration(m.Timeout) * time.Second,
	}
	return nil
}

// Gather fetches instance-id, instance-type, placement/region, and local-ipv4
// from the cloud metadata service and emits them as fields.
func (m *MetadataInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	fields := make(map[string]interface{})

	metadataPaths := map[string]string{
		"instance-id":   "instance-id",
		"instance-type": "instance-type",
		"region":        "placement/region",
		"local-ipv4":    "local-ipv4",
	}

	for field, path := range metadataPaths {
		val, err := m.fetchMetadata(ctx, path)
		if err != nil {
			return fmt.Errorf("cloud_metadata: failed to fetch %s: %w", field, err)
		}
		fields[field] = val
	}

	acc.AddFields("cloud_metadata", nil, fields)
	return nil
}

// SampleConfig returns the sample configuration for the plugin.
func (m *MetadataInput) SampleConfig() string {
	return `
  ## Cloud metadata service URL.
  ## Default: "http://169.254.169.254/latest/meta-data/"
  # metadata_url = "http://169.254.169.254/latest/meta-data/"

  ## Timeout in seconds for metadata requests.
  ## Default: 2
  # timeout = 2
`
}

// fetchMetadata makes an HTTP GET request to the metadata service for the given path.
func (m *MetadataInput) fetchMetadata(ctx context.Context, path string) (string, error) {
	url := strings.TrimRight(m.metadataURL, "/") + "/" + strings.TrimLeft(path, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}
