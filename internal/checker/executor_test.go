package checker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutorExecuteAllPass(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&alwaysPassChecker{})

	exec := NewExecutor(reg, zerolog.Nop())
	req := &pb.HealthCheckRequest{
		RequestId:      "req-1",
		TimeoutSeconds: 10,
		Items: []*pb.CheckItem{
			{Id: "item-1", Type: "always_pass", Category: "test", Name: "Always Pass"},
		},
	}

	var results []*pb.HealthCheckResult
	err := exec.Execute(context.Background(), req, func(r *pb.HealthCheckResult) {
		results = append(results, r)
	})
	require.NoError(t, err)

	require.Len(t, results, 2) // 1 intermediate + 1 final
	assert.False(t, results[0].Completed)
	assert.True(t, results[1].Completed)
	assert.Equal(t, int32(1), results[1].Summary.Total)
	assert.Equal(t, int32(1), results[1].Summary.Pass)
}

func TestExecutorUnknownCheckerType(t *testing.T) {
	reg := NewRegistry()
	exec := NewExecutor(reg, zerolog.Nop())
	req := &pb.HealthCheckRequest{
		RequestId: "req-2",
		Items: []*pb.CheckItem{
			{Id: "item-1", Type: "nonexistent", Category: "test", Name: "Missing"},
		},
	}

	var results []*pb.HealthCheckResult
	err := exec.Execute(context.Background(), req, func(r *pb.HealthCheckResult) {
		results = append(results, r)
	})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, pb.CheckStatus_STATUS_ERROR, results[0].Results[0].Status)
}

type alwaysPassChecker struct{}

func (c *alwaysPassChecker) Type() string     { return "always_pass" }
func (c *alwaysPassChecker) Category() string { return "test" }
func (c *alwaysPassChecker) Check(_ context.Context, _ json.RawMessage) (*CheckResult, error) {
	return &CheckResult{
		Status:   StatusPass,
		Message:  "all good",
		Duration: time.Millisecond,
	}, nil
}
