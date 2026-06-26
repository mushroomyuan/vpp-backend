package query

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platformtelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/types"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

type QueryAggregation struct {
	TenantID   string
	CUCode     string
	MetricName string
	StartTime  time.Time
	EndTime    time.Time
	// Step is the downsampling window, e.g. time.Minute, 15*time.Minute.
	Step      time.Duration
	Functions []model.AggFunction
}

type QueryAggregationHandler = decorator.QueryHandler[QueryAggregation, []*model.AggregatedPoint]

type queryAggregationHandler struct {
	aggRepo port.AggregationRepository
}

func NewQueryAggregationHandler(
	aggRepo port.AggregationRepository,
	metricsClient decorator.MetricsClient,
) QueryAggregationHandler {
	if aggRepo == nil {
		panic("NewQueryAggregationHandler: aggRepo is required")
	}
	return decorator.ApplyQueryDecorators[QueryAggregation, []*model.AggregatedPoint](
		queryAggregationHandler{aggRepo: aggRepo},
		metricsClient,
	)
}

func (h queryAggregationHandler) Handle(ctx context.Context, q QueryAggregation) ([]*model.AggregatedPoint, error) {
	ctx, span := platformtelemetry.Start(ctx, "query_aggregation")
	defer span.End()

	// Apply the same 30-day window policy as raw-record queries.
	if q.EndTime.Sub(q.StartTime) > maxQueryRange {
		return nil, fmt.Errorf("%w (requested: %v)", types.ErrQueryRangeExceeded, q.EndTime.Sub(q.StartTime))
	}

	domainQuery := model.AggregationQuery{
		TenantID:   q.TenantID,
		CUCode:     q.CUCode,
		MetricName: q.MetricName,
		StartTime:  q.StartTime,
		EndTime:    q.EndTime,
		Step:       q.Step,
		Functions:  q.Functions,
	}
	if err := domainQuery.Validate(); err != nil {
		return nil, err
	}
	return h.aggRepo.Query(ctx, domainQuery)
}
