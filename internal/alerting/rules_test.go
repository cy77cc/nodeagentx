package alerting

import (
	"testing"
	"time"
)

func TestAlertConditionEvaluate(t *testing.T) {
	tests := []struct {
		name      string
		condition AlertCondition
		value     float64
		want      bool
	}{
		{name: "greater than - true", condition: AlertCondition{Operator: ">", Threshold: 80}, value: 90, want: true},
		{name: "greater than - false", condition: AlertCondition{Operator: ">", Threshold: 80}, value: 70, want: false},
		{name: "greater than - equal", condition: AlertCondition{Operator: ">", Threshold: 80}, value: 80, want: false},
		{name: "less than - true", condition: AlertCondition{Operator: "<", Threshold: 80}, value: 70, want: true},
		{name: "less than - false", condition: AlertCondition{Operator: "<", Threshold: 80}, value: 90, want: false},
		{name: "less than - equal", condition: AlertCondition{Operator: "<", Threshold: 80}, value: 80, want: false},
		{name: "greater or equal - true greater", condition: AlertCondition{Operator: ">=", Threshold: 80}, value: 90, want: true},
		{name: "greater or equal - true equal", condition: AlertCondition{Operator: ">=", Threshold: 80}, value: 80, want: true},
		{name: "greater or equal - false", condition: AlertCondition{Operator: ">=", Threshold: 80}, value: 70, want: false},
		{name: "less or equal - true less", condition: AlertCondition{Operator: "<=", Threshold: 80}, value: 70, want: true},
		{name: "less or equal - true equal", condition: AlertCondition{Operator: "<=", Threshold: 80}, value: 80, want: true},
		{name: "less or equal - false", condition: AlertCondition{Operator: "<=", Threshold: 80}, value: 90, want: false},
		{name: "equal - true", condition: AlertCondition{Operator: "==", Threshold: 80}, value: 80, want: true},
		{name: "equal - false", condition: AlertCondition{Operator: "==", Threshold: 80}, value: 90, want: false},
		{name: "not equal - true", condition: AlertCondition{Operator: "!=", Threshold: 80}, value: 90, want: true},
		{name: "not equal - false", condition: AlertCondition{Operator: "!=", Threshold: 80}, value: 80, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.condition.Evaluate(tt.value)
			if got != tt.want {
				t.Errorf("Evaluate(%v) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestAlertStateMachine(t *testing.T) {
	now := time.Now()

	t.Run("OK to PENDING when condition met", func(t *testing.T) {
		rule := &EvaluatedRule{
			Name:      "test",
			Condition: AlertCondition{Operator: ">", Threshold: 80},
			Duration:  5 * time.Minute,
			State:     StateOK,
		}

		rule.ConditionMet(true, now)
		if rule.State != StatePending {
			t.Errorf("expected state %s, got %s", StatePending, rule.State)
		}
		if !rule.TriggeredAt.Equal(now) {
			t.Errorf("expected TriggeredAt %v, got %v", now, rule.TriggeredAt)
		}
	})

	t.Run("PENDING to OK when condition not met", func(t *testing.T) {
		rule := &EvaluatedRule{
			Name:       "test",
			Condition:  AlertCondition{Operator: ">", Threshold: 80},
			Duration:   5 * time.Minute,
			State:      StatePending,
			TriggeredAt: now.Add(-2 * time.Minute),
		}

		rule.ConditionMet(false, now)
		if rule.State != StateOK {
			t.Errorf("expected state %s, got %s", StateOK, rule.State)
		}
	})

	t.Run("PENDING stays PENDING when condition met but duration not met", func(t *testing.T) {
		rule := &EvaluatedRule{
			Name:       "test",
			Condition:  AlertCondition{Operator: ">", Threshold: 80},
			Duration:   5 * time.Minute,
			State:      StatePending,
			TriggeredAt: now.Add(-2 * time.Minute),
		}

		rule.ConditionMet(true, now)
		if rule.State != StatePending {
			t.Errorf("expected state %s, got %s", StatePending, rule.State)
		}
	})

	t.Run("PENDING to FIRING when condition met and duration met", func(t *testing.T) {
		rule := &EvaluatedRule{
			Name:       "test",
			Condition:  AlertCondition{Operator: ">", Threshold: 80},
			Duration:   5 * time.Minute,
			State:      StatePending,
			TriggeredAt: now.Add(-6 * time.Minute),
		}

		rule.ConditionMet(true, now)
		if rule.State != StateFiring {
			t.Errorf("expected state %s, got %s", StateFiring, rule.State)
		}
	})

	t.Run("FIRING to OK when condition not met", func(t *testing.T) {
		rule := &EvaluatedRule{
			Name:      "test",
			Condition: AlertCondition{Operator: ">", Threshold: 80},
			Duration:  5 * time.Minute,
			State:     StateFiring,
		}

		rule.ConditionMet(false, now)
		if rule.State != StateOK {
			t.Errorf("expected state %s, got %s", StateOK, rule.State)
		}
	})

	t.Run("OK stays OK when condition not met", func(t *testing.T) {
		rule := &EvaluatedRule{
			Name:      "test",
			Condition: AlertCondition{Operator: ">", Threshold: 80},
			Duration:  5 * time.Minute,
			State:     StateOK,
		}

		rule.ConditionMet(false, now)
		if rule.State != StateOK {
			t.Errorf("expected state %s, got %s", StateOK, rule.State)
		}
	})
}
