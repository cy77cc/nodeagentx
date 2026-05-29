package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func init() {
	collector.RegisterInput("http", func() collector.Input {
		return &HTTPInput{
			Method:  "GET",
			Timeout: 5,
		}
	})
}

// HTTPInput polls HTTP endpoints and emits response metrics.
type HTTPInput struct {
	URLs    []string
	Method  string
	Timeout int
	client  *http.Client
}

// Init parses the plugin configuration.
func (h *HTTPInput) Init(cfg map[string]interface{}) error {
	if v, ok := cfg["urls"]; ok {
		switch urls := v.(type) {
		case []string:
			h.URLs = urls
		case []interface{}:
			for _, u := range urls {
				s, ok := u.(string)
				if !ok {
					return fmt.Errorf("http: url must be a string, got %T", u)
				}
				h.URLs = append(h.URLs, s)
			}
		default:
			return fmt.Errorf("http: urls must be a list of strings, got %T", v)
		}
	}

	if v, ok := cfg["method"]; ok {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("http: method must be a string, got %T", v)
		}
		h.Method = s
	}

	if v, ok := cfg["timeout"]; ok {
		switch t := v.(type) {
		case int:
			h.Timeout = t
		case int64:
			h.Timeout = int(t)
		case float64:
			h.Timeout = int(t)
		default:
			return fmt.Errorf("http: timeout must be a number, got %T", v)
		}
	}

	h.client = &http.Client{
		Timeout: time.Duration(h.Timeout) * time.Second,
	}

	return nil
}

// Gather makes HTTP requests to configured URLs and emits metrics.
func (h *HTTPInput) Gather(ctx context.Context, acc collector.Accumulator) error {
	for _, url := range h.URLs {
		tags := map[string]string{
			"url":    url,
			"method": h.Method,
		}

		req, err := http.NewRequestWithContext(ctx, h.Method, url, nil)
		if err != nil {
			fields := map[string]interface{}{
				"error": err.Error(),
			}
			acc.AddFields("http", tags, fields)
			continue
		}

		start := time.Now()
		resp, err := h.client.Do(req)
		elapsed := time.Since(start)

		if err != nil {
			fields := map[string]interface{}{
				"error": err.Error(),
			}
			acc.AddFields("http", tags, fields)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		fields := map[string]interface{}{
			"status_code":     resp.StatusCode,
			"response_time_ms": elapsed.Milliseconds(),
			"content_length":  len(body),
		}
		acc.AddFields("http", tags, fields)
	}

	return nil
}

// SampleConfig returns the plugin's sample configuration.
func (h *HTTPInput) SampleConfig() string {
	return `
  ## List of HTTP endpoints to poll
  # urls = ["http://localhost:8080/health"]

  ## HTTP method to use
  # method = "GET"

  ## Request timeout in seconds
  # timeout = 5
`
}
