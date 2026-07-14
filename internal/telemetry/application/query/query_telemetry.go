package query

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/telemetry/application/types"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// maxQueryRange is an application-layer policy that prevents runaway TSDB scans.
// It is deliberately NOT in the domain: QueryCondition.Validate() only enforces
// structural invariants (non-empty fields, time ordering).
const maxQueryRange = 30 * 24 * time.Hour

type QueryTelemetry struct {
	TenantID   string
	CUCode     string
	MetricName string
	StartTime  time.Time
	EndTime    time.Time
}

type QueryTelemetryHandler = decorator.QueryHandler[QueryTelemetry, []*model.TelemetryRecord]

type queryTelemetryHandler struct {
	telemetryRepo port.TelemetryRepository
}

func NewQueryTelemetryHandler(
	telemetryRepo port.TelemetryRepository,
	metricsClient decorator.MetricsClient,
) QueryTelemetryHandler {
	if telemetryRepo == nil {
		panic("NewQueryTelemetryHandler: telemetryRepo is required")
	}
	return decorator.ApplyQueryDecorators[QueryTelemetry, []*model.TelemetryRecord](
		queryTelemetryHandler{telemetryRepo: telemetryRepo},
		metricsClient,
	)
}

func (h queryTelemetryHandler) Handle(ctx context.Context, q QueryTelemetry) ([]*model.TelemetryRecord, error) {
	if q.EndTime.Sub(q.StartTime) > maxQueryRange {
		return nil, fmt.Errorf("%w (requested: %v)", types.ErrQueryRangeExceeded, q.EndTime.Sub(q.StartTime))
	}

	cond := model.NewQueryCondition(q.TenantID, q.CUCode, q.MetricName, q.StartTime, q.EndTime)
	if err := cond.Validate(); err != nil {
		return nil, err
	}
	return h.telemetryRepo.Query(ctx, cond)
}
