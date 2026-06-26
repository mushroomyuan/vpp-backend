package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// AggregationRepository provides read access to pre-aggregated or on-demand
// downsampled time-series data. Implementations are expected to push the
// Step and Functions down to the storage engine (e.g. TimescaleDB time_bucket,
// InfluxDB FLUX aggregateWindow) rather than fetching raw rows.
type AggregationRepository interface {
	Query(ctx context.Context, query model.AggregationQuery) ([]*model.AggregatedPoint, error)
}
