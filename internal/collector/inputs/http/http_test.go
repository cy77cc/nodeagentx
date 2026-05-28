package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestHTTPInputInit(t *testing.T) {
	tests := []struct {
		name       string
		cfg        map[string]interface{}
		wantErr    bool
		wantURLs   []string
		wantMethod string
		wantTimeout int
	}{
		{
			name:        "defaults",
			cfg:         map[string]interface{}{},
			wantMethod:  "GET",
			wantTimeout: 5,
		},
		{
			name: "full config",
			cfg: map[string]interface{}{
				"urls":    []interface{}{"http://localhost:8080"},
				"method":  "POST",
				"timeout": 10,
			},
			wantURLs:    []string{"http://localhost:8080"},
			wantMethod:  "POST",
			wantTimeout: 10,
		},
		{
			name: "string slice urls",
			cfg: map[string]interface{}{
				"urls": []string{"http://a.com", "http://b.com"},
			},
			wantURLs:    []string{"http://a.com", "http://b.com"},
			wantMethod:  "GET",
			wantTimeout: 5,
		},
		{
			name: "invalid urls type",
			cfg: map[string]interface{}{
				"urls": "not-a-list",
			},
			wantErr: true,
		},
		{
			name: "invalid method type",
			cfg: map[string]interface{}{
				"method": 123,
			},
			wantErr: true,
		},
		{
			name: "invalid timeout type",
			cfg: map[string]interface{}{
				"timeout": "notanumber",
			},
			wantErr: true,
		},
		{
			name: "float64 timeout from yaml",
			cfg: map[string]interface{}{
				"timeout": float64(15),
			},
			wantMethod:  "GET",
			wantTimeout: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &HTTPInput{
				Method:  "GET",
				Timeout: 5,
			}
			err := input.Init(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Init() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if input.Method != tt.wantMethod {
				t.Errorf("Method = %q, want %q", input.Method, tt.wantMethod)
			}
			if input.Timeout != tt.wantTimeout {
				t.Errorf("Timeout = %d, want %d", input.Timeout, tt.wantTimeout)
			}
			if tt.wantURLs != nil {
				if len(input.URLs) != len(tt.wantURLs) {
					t.Fatalf("URLs length = %d, want %d", len(input.URLs), len(tt.wantURLs))
				}
				for i, u := range input.URLs {
					if u != tt.wantURLs[i] {
						t.Errorf("URLs[%d] = %q, want %q", i, u, tt.wantURLs[i])
					}
				}
			}
		})
	}
}

// newTestServer creates an httptest.Server returning the given status and body.
func newTestServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
}

func TestHTTPInputGather(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "200 OK",
			status:     http.StatusOK,
			body:       "hello",
			wantStatus: http.StatusOK,
		},
		{
			name:       "404 Not Found",
			status:     http.StatusNotFound,
			body:       "not found",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "500 Internal Server Error",
			status:     http.StatusInternalServerError,
			body:       "error",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := newTestServer(tt.status, tt.body)
			defer ts.Close()

			input := &HTTPInput{
				Method:  "GET",
				Timeout: 5,
			}
			if err := input.Init(map[string]interface{}{
				"urls": []interface{}{ts.URL},
			}); err != nil {
				t.Fatalf("Init() error: %v", err)
			}

			acc := collector.NewAccumulator(10)
			if err := input.Gather(context.Background(), acc); err != nil {
				t.Fatalf("Gather() error: %v", err)
			}

			metrics := acc.Collect()
			if len(metrics) == 0 {
				t.Fatal("Gather() produced 0 metrics, want at least 1")
			}

			m := metrics[0]
			if m.Name() != "http" {
				t.Errorf("metric name = %q, want %q", m.Name(), "http")
			}

			tags := m.Tags()
			if tags["url"] != ts.URL {
				t.Errorf("tag url = %q, want %q", tags["url"], ts.URL)
			}
			if tags["method"] != "GET" {
				t.Errorf("tag method = %q, want %q", tags["method"], "GET")
			}

			fields := m.Fields()
			if fields["status_code"] != tt.wantStatus {
				t.Errorf("field status_code = %v, want %d", fields["status_code"], tt.wantStatus)
			}
			if _, ok := fields["response_time_ms"]; !ok {
				t.Error("missing 'response_time_ms' field")
			}
			if _, ok := fields["content_length"]; !ok {
				t.Error("missing 'content_length' field")
			}
			if fields["content_length"] != len(tt.body) {
				t.Errorf("content_length = %v, want %d", fields["content_length"], len(tt.body))
			}
		})
	}
}

func TestHTTPInputGatherError(t *testing.T) {
	input := &HTTPInput{
		Method:  "GET",
		Timeout: 5,
	}
	if err := input.Init(map[string]interface{}{
		"urls": []interface{}{"http://127.0.0.1:1"},
	}); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	acc := collector.NewAccumulator(10)
	// Use background context; the request will fail because nothing listens on port 1.
	if err := input.Gather(context.Background(), acc); err != nil {
		t.Fatalf("Gather() error: %v", err)
	}

	metrics := acc.Collect()
	if len(metrics) == 0 {
		t.Fatal("expected at least 1 metric for error case")
	}

	m := metrics[0]
	fields := m.Fields()
	if _, ok := fields["error"]; !ok {
		t.Error("expected 'error' field for failed request")
	}
	if _, ok := fields["status_code"]; ok {
		t.Error("should not have 'status_code' field on error")
	}
}

func TestHTTPInputSampleConfig(t *testing.T) {
	input := &HTTPInput{}
	sc := input.SampleConfig()
	if sc == "" {
		t.Error("SampleConfig() should not be empty")
	}
}
