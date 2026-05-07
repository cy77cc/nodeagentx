package checker

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
)

// CheckStatus represents the outcome of a single check.
type CheckStatus int

const (
	StatusPass  CheckStatus = iota
	StatusFail
	StatusWarn
	StatusError
	StatusSkip
)

func (s CheckStatus) ToProto() pb.CheckStatus {
	switch s {
	case StatusPass:
		return pb.CheckStatus_STATUS_PASS
	case StatusFail:
		return pb.CheckStatus_STATUS_FAIL
	case StatusWarn:
		return pb.CheckStatus_STATUS_WARN
	case StatusError:
		return pb.CheckStatus_STATUS_ERROR
	case StatusSkip:
		return pb.CheckStatus_STATUS_SKIP
	default:
		return pb.CheckStatus_STATUS_ERROR
	}
}

// CheckSeverity indicates the risk level of a check finding.
type CheckSeverity int

const (
	SeverityInfo     CheckSeverity = iota
	SeverityLow
	SeverityMedium
	SeverityHigh
	SeverityCritical
)

func (s CheckSeverity) ToProto() pb.CheckSeverity {
	switch s {
	case SeverityInfo:
		return pb.CheckSeverity_SEVERITY_INFO
	case SeverityLow:
		return pb.CheckSeverity_SEVERITY_LOW
	case SeverityMedium:
		return pb.CheckSeverity_SEVERITY_MEDIUM
	case SeverityHigh:
		return pb.CheckSeverity_SEVERITY_HIGH
	case SeverityCritical:
		return pb.CheckSeverity_SEVERITY_CRITICAL
	default:
		return pb.CheckSeverity_SEVERITY_INFO
	}
}

// CheckResult holds the outcome of a single check item.
type CheckResult struct {
	ItemID        string
	Type          string
	Name          string
	Status        CheckStatus
	ActualValue   string
	ExpectedValue string
	Message       string
	Remediation   string
	Severity      CheckSeverity
	Duration      time.Duration
}

func (r *CheckResult) ToProto() *pb.CheckResult {
	return &pb.CheckResult{
		ItemId:        r.ItemID,
		Type:          r.Type,
		Name:          r.Name,
		Status:        r.Status.ToProto(),
		ActualValue:   r.ActualValue,
		ExpectedValue: r.ExpectedValue,
		Message:       r.Message,
		Remediation:   r.Remediation,
		Severity:      r.Severity.ToProto(),
		DurationMs:    r.Duration.Milliseconds(),
	}
}

// Checker is the interface for a single health check type.
type Checker interface {
	Type() string
	Category() string
	Check(ctx context.Context, params json.RawMessage) (*CheckResult, error)
}

// BuildSummary computes a CheckSummary from a slice of CheckResults.
func BuildSummary(results []*CheckResult, totalDuration time.Duration) *pb.CheckSummary {
	s := &pb.CheckSummary{
		Total:           int32(len(results)),
		TotalDurationMs: totalDuration.Milliseconds(),
	}
	for _, r := range results {
		switch r.Status {
		case StatusPass:
			s.Pass++
		case StatusFail:
			s.Fail++
		case StatusWarn:
			s.Warn++
		case StatusError:
			s.Error++
		case StatusSkip:
			s.Skip++
		}
	}
	return s
}
