package alerting

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/cy77cc/opsagent/internal/collector"
)

// conditionPattern parses condition strings like "cpu_usage_percent > 80".
var conditionPattern = regexp.MustCompile(`^(\S+)\s*(>|<|>=|<=|==|!=)\s*(.+)$`)

// RuleConfig is the configuration for a single alerting rule.
type RuleConfig struct {
	Name     string `json:"name"`
	Condition string `json:"condition"`
	Severity  string `json:"severity"`
	For       string `json:"for"`
}

// EngineConfig holds the full alerting engine configuration.
type EngineConfig struct {
	Rules []RuleConfig `json:"rules"`
}

// Engine evaluates alerting rules against incoming metrics.
type Engine struct {
	rules     []*EvaluatedRule
	notifiers []Notifier
}

// NewEngine creates an Engine from the given configuration.
func NewEngine(cfg EngineConfig) *Engine {
	e := &Engine{}
	for _, rc := range cfg.Rules {
		cond, err := parseCondition(rc.Condition)
		if err != nil {
			continue
		}

		d := time.Duration(0)
		if rc.For != "" && rc.For != "0s" {
			d, _ = time.ParseDuration(rc.For)
		}

		e.rules = append(e.rules, &EvaluatedRule{
			Name:      rc.Name,
			Condition: cond,
			Severity:  rc.Severity,
			Duration:  d,
			State:     StateOK,
		})
	}
	return e
}

// AddNotifier registers a notifier for firing alerts.
func (e *Engine) AddNotifier(n Notifier) {
	e.notifiers = append(e.notifiers, n)
}

// Evaluate checks all rules against the provided metrics and returns newly firing alerts.
func (e *Engine) Evaluate(metrics []*collector.Metric) []Alert {
	// Build value map: key is name_fieldKey
	values := make(map[string]float64)
	for _, m := range metrics {
		for k, v := range m.Fields() {
			key := m.Name() + "_" + k
			switch val := v.(type) {
			case float64:
				values[key] = val
			case int64:
				values[key] = float64(val)
			case int:
				values[key] = float64(val)
			}
		}
	}

	now := time.Now()
	var alerts []Alert

	for _, rule := range e.rules {
		prevState := rule.State
		val, ok := values[rule.Condition.Metric]
		if !ok {
			rule.ConditionMet(false, now)
			continue
		}

		rule.CurrentValue = val
		met := rule.Condition.Evaluate(val)
		rule.ConditionMet(met, now)

		// Only emit alert on transition to FIRING
		if rule.State == StateFiring && prevState != StateFiring {
			alert := Alert{
				Name:         rule.Name,
				State:        StateFiring,
				Severity:     rule.Severity,
				CurrentValue: rule.CurrentValue,
				Threshold:    rule.Condition.Threshold,
				Message:      fmt.Sprintf("%s: %s is %v (threshold: %v)", rule.Name, rule.Condition.Metric, rule.CurrentValue, rule.Condition.Threshold),
			}
			alerts = append(alerts, alert)

			for _, n := range e.notifiers {
				_ = n.Notify(alert)
			}
		}
	}

	return alerts
}

// parseCondition parses a condition string like "cpu_usage_percent > 80".
func parseCondition(s string) (AlertCondition, error) {
	matches := conditionPattern.FindStringSubmatch(s)
	if matches == nil {
		return AlertCondition{}, fmt.Errorf("invalid condition: %s", s)
	}

	threshold, err := strconv.ParseFloat(matches[3], 64)
	if err != nil {
		return AlertCondition{}, fmt.Errorf("invalid threshold: %s", matches[3])
	}

	return AlertCondition{
		Metric:    matches[1],
		Operator:  matches[2],
		Threshold: threshold,
	}, nil
}
