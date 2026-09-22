package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

// CachePort stores the latest forecast batch for one
// (tenant, CU, metric). Redis is a hot cache of the whole horizon
// (design plan §7.1); miss is not an error — GetLatestPrediction
// falls back to HistoryPort.GetLatestBatch.
//
// GetLatestBatch returns (nil, nil) on a miss. Callers run
// service.SelectNextPoint on the batch; they must not treat a hit
// whose points all lie in the past as a cache hit that answers the
// request (that batch still falls through to Postgres).
type CachePort interface {
	GetLatestBatch(ctx context.Context, tenantID, cuCode, metricName string) ([]model.Prediction, error)
	SetLatestBatch(ctx context.Context, batch []model.Prediction) error
}
