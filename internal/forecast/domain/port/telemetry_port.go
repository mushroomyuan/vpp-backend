package port

import (
	"context"
	"time"
)

// AggregatedPoint is Forecast's read model for one Telemetry
// QueryAggregation bucket. Avg feeds MovingAveragePredictor; Last feeds
// SamePeriodPriorPredictor. Pointers are nil when the function was not
// returned (empty window). Domain code must not import the telemetry
// proto — that conversion belongs to adapter/outbound/telemetry_grpc.
type AggregatedPoint struct {
	Timestamp time.Time
	Avg       *float64
	Last      *float64
}

// TelemetryPort is the only Telemetry capability Forecast needs.
// QueryAggregation maps 1:1 onto telemetrypb.QueryAggregation; the
// adapter always requests AVG and LAST. v1 does not need Telemetry to
// grow any new RPCs.
type TelemetryPort interface {
	QueryAggregation(ctx context.Context, tenantID, cuCode, metricName string,
		start, end time.Time, stepSeconds int64) ([]AggregatedPoint, error)
}
