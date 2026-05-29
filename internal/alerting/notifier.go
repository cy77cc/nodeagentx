package alerting

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier sends alert notifications.
type Notifier interface {
	Notify(alert Alert) error
}

// WebhookNotifier sends alerts via HTTP POST to a webhook URL.
type WebhookNotifier struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// NewWebhookNotifier creates a WebhookNotifier with the given URL and headers.
func NewWebhookNotifier(url string, headers map[string]string) *WebhookNotifier {
	return &WebhookNotifier{
		url:     url,
		headers: headers,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Notify sends the alert as JSON to the webhook URL.
func (w *WebhookNotifier) Notify(alert Alert) error {
	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
