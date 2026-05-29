package alerting

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookNotifier(t *testing.T) {
	var receivedBody Alert

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-Custom") != "test-value" {
			t.Errorf("expected header X-Custom=test-value, got %s", r.Header.Get("X-Custom"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &receivedBody); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	headers := map[string]string{
		"X-Custom": "test-value",
	}
	notifier := NewWebhookNotifier(server.URL, headers)

	alert := Alert{
		Name:         "test_alert",
		State:        StateFiring,
		Severity:     "critical",
		CurrentValue: 95.0,
		Threshold:    80.0,
		Message:      "CPU usage is high",
	}

	err := notifier.Notify(alert)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedBody.Name != alert.Name {
		t.Errorf("expected name %s, got %s", alert.Name, receivedBody.Name)
	}
	if receivedBody.State != alert.State {
		t.Errorf("expected state %s, got %s", alert.State, receivedBody.State)
	}
	if receivedBody.Severity != alert.Severity {
		t.Errorf("expected severity %s, got %s", alert.Severity, receivedBody.Severity)
	}
	if receivedBody.CurrentValue != alert.CurrentValue {
		t.Errorf("expected currentValue %v, got %v", alert.CurrentValue, receivedBody.CurrentValue)
	}
	if receivedBody.Threshold != alert.Threshold {
		t.Errorf("expected threshold %v, got %v", alert.Threshold, receivedBody.Threshold)
	}
	if receivedBody.Message != alert.Message {
		t.Errorf("expected message %s, got %s", alert.Message, receivedBody.Message)
	}
}

func TestWebhookNotifierError(t *testing.T) {
	notifier := NewWebhookNotifier("http://invalid-host-that-does-not-exist.local", nil)
	alert := Alert{Name: "test", State: StateFiring, Severity: "high"}
	err := notifier.Notify(alert)
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}
