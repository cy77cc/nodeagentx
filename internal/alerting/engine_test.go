package alerting

import (
	"testing"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

func TestEngineEvaluate(t *testing.T) {
	cfg := EngineConfig{
		Rules: []RuleConfig{
			{
				Name:     "high_cpu",
				Condition: "cpu_usage_percent > 80",
				Severity: "critical",
				For:      "0s",
			},
		},
	}

	engine := NewEngine(cfg)

	fields := map[string]interface{}{
		"usage_percent": 95.0,
	}
	metrics := []*collector.Metric{
		collector.NewMetric("cpu", nil, fields, collector.Gauge, time.Time{}),
	}

	alerts := engine.Evaluate(metrics)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Name != "high_cpu" {
		t.Errorf("expected alert name 'high_cpu', got %s", alerts[0].Name)
	}
	if alerts[0].State != StateFiring {
		t.Errorf("expected state %s, got %s", StateFiring, alerts[0].State)
	}
	if alerts[0].Severity != "critical" {
		t.Errorf("expected severity 'critical', got %s", alerts[0].Severity)
	}
}

func TestEngineNoAlert(t *testing.T) {
	cfg := EngineConfig{
		Rules: []RuleConfig{
			{
				Name:     "high_cpu",
				Condition: "cpu_usage_percent > 80",
				Severity: "critical",
				For:      "0s",
			},
		},
	}

	engine := NewEngine(cfg)

	fields := map[string]interface{}{
		"usage_percent": 50.0,
	}
	metrics := []*collector.Metric{
		collector.NewMetric("cpu", nil, fields, collector.Gauge, time.Time{}),
	}

	alerts := engine.Evaluate(metrics)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestEngineWithPendingDuration(t *testing.T) {
	cfg := EngineConfig{
		Rules: []RuleConfig{
			{
				Name:     "high_cpu",
				Condition: "cpu_usage_percent > 80",
				Severity: "critical",
				For:      "5m",
			},
		},
	}

	engine := NewEngine(cfg)

	fields := map[string]interface{}{
		"usage_percent": 95.0,
	}
	metrics := []*collector.Metric{
		collector.NewMetric("cpu", nil, fields, collector.Gauge, time.Time{}),
	}

	// First evaluation - should go to PENDING, no alert
	alerts := engine.Evaluate(metrics)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts on first eval, got %d", len(alerts))
	}
	if engine.rules[0].State != StatePending {
		t.Errorf("expected state %s, got %s", StatePending, engine.rules[0].State)
	}
}

func TestEngineAddNotifier(t *testing.T) {
	engine := NewEngine(EngineConfig{})
	n := &mockNotifier{}
	engine.AddNotifier(n)
	if len(engine.notifiers) != 1 {
		t.Errorf("expected 1 notifier, got %d", len(engine.notifiers))
	}
}

type mockNotifier struct {
	alerts []Alert
}

func (m *mockNotifier) Notify(alert Alert) error {
	m.alerts = append(m.alerts, alert)
	return nil
}
