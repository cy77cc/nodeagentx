package logparse

import (
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestLogParseProcessorInit(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "message",
				"parser":       "grok",
				"grok_pattern": `%{IP:client} %{GREEDYDATA:msg}`,
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if len(p.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(p.Rules))
	}
	if p.Rules[0].Field != "message" {
		t.Errorf("rule field = %q, want %q", p.Rules[0].Field, "message")
	}
}

func TestLogParseApplyGrok(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "message",
				"parser":       "grok",
				"grok_pattern": `%{IP:client} %{WORD:method} %{GREEDYDATA:path}`,
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	m := collector.NewMetric("access_log",
		map[string]string{},
		map[string]interface{}{
			"message": "192.168.1.1 GET /index.html",
		},
		collector.Gauge, time.Time{})

	result := p.Apply([]*collector.Metric{m})
	if len(result) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(result))
	}

	fields := result[0].Fields()
	if fields["client"] != "192.168.1.1" {
		t.Errorf("client = %v, want %q", fields["client"], "192.168.1.1")
	}
	if fields["method"] != "GET" {
		t.Errorf("method = %v, want %q", fields["method"], "GET")
	}
	if fields["path"] != "/index.html" {
		t.Errorf("path = %v, want %q", fields["path"], "/index.html")
	}
}

func TestLogParseApplyJSON(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":  "message",
				"parser": "json",
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	m := collector.NewMetric("app_log",
		map[string]string{},
		map[string]interface{}{
			"message": `{"status":200,"latency_ms":42.5,"path":"/api"}`,
		},
		collector.Gauge, time.Time{})

	result := p.Apply([]*collector.Metric{m})
	if len(result) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(result))
	}

	fields := result[0].Fields()
	// JSON numbers are float64 by default in Go's encoding/json
	if fields["status"] != float64(200) {
		t.Errorf("status = %v (%T), want float64(200)", fields["status"], fields["status"])
	}
	if fields["latency_ms"] != 42.5 {
		t.Errorf("latency_ms = %v, want 42.5", fields["latency_ms"])
	}
	if fields["path"] != "/api" {
		t.Errorf("path = %v, want %q", fields["path"], "/api")
	}
}

func TestLogParseApplyRegex(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":         "message",
				"parser":        "regex",
				"regex_pattern": `^(?P<host>\S+) (?P<user>\S+)$`,
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	m := collector.NewMetric("login",
		map[string]string{},
		map[string]interface{}{
			"message": "server-01 admin",
		},
		collector.Gauge, time.Time{})

	result := p.Apply([]*collector.Metric{m})
	if len(result) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(result))
	}

	fields := result[0].Fields()
	if fields["host"] != "server-01" {
		t.Errorf("host = %v, want %q", fields["host"], "server-01")
	}
	if fields["user"] != "admin" {
		t.Errorf("user = %v, want %q", fields["user"], "admin")
	}
}

func TestLogParseSampleConfig(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := p.SampleConfig()
	if cfg == "" {
		t.Error("expected non-empty sample config")
	}
}

func TestLogParseApplyMultipleRules(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "message",
				"parser":       "grok",
				"grok_pattern": `%{IP:client} %{WORD:method}`,
			},
			map[string]interface{}{
				"field":         "client",
				"parser":        "regex",
				"regex_pattern": `^(?P<first_octet>\d+)\.\d+\.\d+\.\d+$`,
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	m := collector.NewMetric("access",
		map[string]string{},
		map[string]interface{}{
			"message": "10.0.0.1 GET",
		},
		collector.Gauge, time.Time{})

	result := p.Apply([]*collector.Metric{m})
	fields := result[0].Fields()

	// First rule extracts client and method from message
	if fields["client"] != "10.0.0.1" {
		t.Errorf("client = %v, want %q", fields["client"], "10.0.0.1")
	}
	if fields["method"] != "GET" {
		t.Errorf("method = %v, want %q", fields["method"], "GET")
	}
	// Second rule extracts first_octet from client field
	if fields["first_octet"] != "10" {
		t.Errorf("first_octet = %v, want %q", fields["first_octet"], "10")
	}
}

func TestLogParseApplyMissingField(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "nonexistent",
				"parser":       "grok",
				"grok_pattern": `%{IP:client}`,
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	m := collector.NewMetric("test",
		map[string]string{},
		map[string]interface{}{"value": float64(1)},
		collector.Gauge, time.Time{})

	result := p.Apply([]*collector.Metric{m})
	if len(result) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(result))
	}
	// Metric should be unchanged
	fields := result[0].Fields()
	if _, ok := fields["client"]; ok {
		t.Error("expected no 'client' field when source field is missing")
	}
}

func TestLogParseApplyNoMatch(t *testing.T) {
	p := &LogParseProcessor{}
	cfg := map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"field":        "message",
				"parser":       "grok",
				"grok_pattern": `%{IP:client}`,
			},
		},
	}
	if err := p.Init(cfg); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	m := collector.NewMetric("test",
		map[string]string{},
		map[string]interface{}{
			"message": "not an ip address",
		},
		collector.Gauge, time.Time{})

	result := p.Apply([]*collector.Metric{m})
	fields := result[0].Fields()
	// Should not add a client field since there was no match
	if _, ok := fields["client"]; ok {
		t.Error("expected no 'client' field when grok does not match")
	}
	// Original message should be preserved
	if fields["message"] != "not an ip address" {
		t.Errorf("message should be preserved, got %v", fields["message"])
	}
}

func TestLogParseRegisteredInDefaultRegistry(t *testing.T) {
	f, ok := collector.DefaultRegistry.GetProcessor("logparse")
	if !ok {
		t.Fatal("logparse processor not registered in default registry")
	}
	p := f()
	if p == nil {
		t.Fatal("expected non-nil processor from factory")
	}
}
