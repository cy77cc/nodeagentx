package checker

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"

	pb "github.com/cy77cc/opsagent/internal/grpcclient/proto"
)

// Executor runs health check requests by routing items to registered checkers.
type Executor struct {
	registry *Registry
	logger   zerolog.Logger
}

// NewExecutor creates an Executor.
func NewExecutor(registry *Registry, logger zerolog.Logger) *Executor {
	return &Executor{registry: registry, logger: logger}
}

// Execute runs all check items in the request, calling callback for each
// intermediate result and once more for the final summary.
func (e *Executor) Execute(ctx context.Context, req *pb.HealthCheckRequest,
	callback func(*pb.HealthCheckResult)) error {

	results := make([]*CheckResult, 0, len(req.Items))
	startAll := time.Now()

	for _, item := range req.Items {
		result := e.executeOne(ctx, item)
		results = append(results, result)

		callback(&pb.HealthCheckResult{
			RequestId: req.RequestId,
			Results:   []*pb.CheckResult{result.ToProto()},
			Completed: false,
		})
	}

	protoResults := make([]*pb.CheckResult, len(results))
	for i, r := range results {
		protoResults[i] = r.ToProto()
	}

	callback(&pb.HealthCheckResult{
		RequestId: req.RequestId,
		Results:   protoResults,
		Summary:   BuildSummary(results, time.Since(startAll)),
		Completed: true,
	})

	return nil
}

func (e *Executor) executeOne(ctx context.Context, item *pb.CheckItem) *CheckResult {
	start := time.Now()

	checker, ok := e.registry.Get(item.Type)
	if !ok {
		return &CheckResult{
			ItemID:   item.Id,
			Type:     item.Type,
			Name:     item.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("unknown checker type: %s", item.Type),
			Severity: CheckSeverity(item.Severity),
			Duration: time.Since(start),
		}
	}

	result, err := checker.Check(ctx, item.Params)
	if err != nil {
		e.logger.Error().Err(err).Str("type", item.Type).Str("item_id", item.Id).Msg("checker failed")
		return &CheckResult{
			ItemID:   item.Id,
			Type:     item.Type,
			Name:     item.Name,
			Status:   StatusError,
			Message:  fmt.Sprintf("check execution error: %v", err),
			Severity: CheckSeverity(item.Severity),
			Duration: time.Since(start),
		}
	}

	result.ItemID = item.Id
	result.Type = item.Type
	result.Name = item.Name
	if result.Duration == 0 {
		result.Duration = time.Since(start)
	}
	return result
}
