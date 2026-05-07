package checker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckStatusToProto(t *testing.T) {
	tests := []struct {
		input    CheckStatus
		expected pb.CheckStatus
	}{
		{StatusPass, pb.CheckStatus_STATUS_PASS},
		{StatusFail, pb.CheckStatus_STATUS_FAIL},
		{StatusWarn, pb.CheckStatus_STATUS_WARN},
		{StatusError, pb.CheckStatus_STATUS_ERROR},
		{StatusSkip, pb.CheckStatus_STATUS_SKIP},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.input.ToProto())
	}
}

func TestCheckSeverityToProto(t *testing.T) {
	tests := []struct {
		input    CheckSeverity
		expected pb.CheckSeverity
	}{
		{SeverityInfo, pb.CheckSeverity_SEVERITY_INFO},
		{SeverityLow, pb.CheckSeverity_SEVERITY_LOW},
		{SeverityMedium, pb.CheckSeverity_SEVERITY_MEDIUM},
		{SeverityHigh, pb.CheckSeverity_SEVERITY_HIGH},
		{SeverityCritical, pb.CheckSeverity_SEVERITY_CRITICAL},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.input.ToProto())
	}
}

func TestCheckResultToProto(t *testing.T) {
	r := &CheckResult{
		ItemID:        "test-1",
		Type:          "sysctl_check",
		Name:          "IP Forward",
		Status:        StatusFail,
		ActualValue:   "1",
		ExpectedValue: "0",
		Message:       "ip_forward is enabled",
		Remediation:   "Set net.ipv4.ip_forward=0",
		Severity:      SeverityHigh,
		Duration:      50 * time.Millisecond,
	}

	proto := r.ToProto()
	assert.Equal(t, "test-1", proto.ItemId)
	assert.Equal(t, "sysctl_check", proto.Type)
	assert.Equal(t, pb.CheckStatus_STATUS_FAIL, proto.Status)
	assert.Equal(t, int64(50), proto.DurationMs)
}

func TestBuildSummary(t *testing.T) {
	results := []*CheckResult{
		{Status: StatusPass},
		{Status: StatusPass},
		{Status: StatusFail},
		{Status: StatusWarn},
		{Status: StatusError},
		{Status: StatusSkip},
	}
	summary := BuildSummary(results, 500*time.Millisecond)
	assert.Equal(t, int32(6), summary.Total)
	assert.Equal(t, int32(2), summary.Pass)
	assert.Equal(t, int32(1), summary.Fail)
	assert.Equal(t, int32(1), summary.Warn)
	assert.Equal(t, int32(1), summary.Error)
	assert.Equal(t, int32(1), summary.Skip)
	assert.Equal(t, int64(500), summary.TotalDurationMs)
}

func TestCheckerInterface(t *testing.T) {
	var _ Checker = &mockChecker{}
}

type mockChecker struct{}

func (m *mockChecker) Type() string     { return "mock" }
func (m *mockChecker) Category() string { return "test" }
func (m *mockChecker) Check(_ context.Context, _ json.RawMessage) (*CheckResult, error) {
	return &CheckResult{Status: StatusPass}, nil
}

func requireChecker(t *testing.T, c Checker) {
	t.Helper()
	require.NotEmpty(t, c.Type())
	require.NotEmpty(t, c.Category())
}
