package alerting

import (
	"time"
)

// Alert states.
const (
	StateOK      = "ok"
	StatePending = "pending"
	StateFiring  = "firing"
)

// AlertCondition defines a threshold-based condition on a metric.
type AlertCondition struct {
	Metric    string
	Operator  string
	Threshold float64
	For       time.Duration
}

// Evaluate returns true if the condition is met for the given value.
func (c AlertCondition) Evaluate(value float64) bool {
	switch c.Operator {
	case ">":
		return value > c.Threshold
	case "<":
		return value < c.Threshold
	case ">=":
		return value >= c.Threshold
	case "<=":
		return value <= c.Threshold
	case "==":
		return value == c.Threshold
	case "!=":
		return value != c.Threshold
	default:
		return false
	}
}

// EvaluatedRule is a rule with runtime state for tracking alert lifecycle.
type EvaluatedRule struct {
	Name        string
	Condition   AlertCondition
	Severity    string
	Duration    time.Duration
	State       string
	TriggeredAt time.Time
	CurrentValue float64
}

// ConditionMet updates the rule state based on whether the condition is met.
func (r *EvaluatedRule) ConditionMet(met bool, now time.Time) {
	switch r.State {
	case StateOK:
		if met {
			if r.Duration == 0 {
				r.State = StateFiring
			} else {
				r.State = StatePending
				r.TriggeredAt = now
			}
		}
	case StatePending:
		if !met {
			r.State = StateOK
			r.TriggeredAt = time.Time{}
		} else if now.Sub(r.TriggeredAt) >= r.Duration {
			r.State = StateFiring
		}
	case StateFiring:
		if !met {
			r.State = StateOK
			r.TriggeredAt = time.Time{}
		}
	}
}

// Alert represents an alert notification payload.
type Alert struct {
	Name         string  `json:"name"`
	State        string  `json:"state"`
	Severity     string  `json:"severity"`
	CurrentValue float64 `json:"current_value"`
	Threshold    float64 `json:"threshold"`
	Message      string  `json:"message"`
}
