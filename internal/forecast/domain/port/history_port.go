package port

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

// HistoryQuery is the window QueryForecastHistory hands to HistoryPort.
// StartTime / EndTime bound target_timestamp (inclusive). GeneratedAt
// zero means "latest generation per target_timestamp"; non-zero means
// only that batch (the "4h-ahead vs 1h-ahead" comparison).
type HistoryQuery struct {
	TenantID    string
	CUCode      string
	MetricName  string
	StartTime   time.Time
	EndTime     time.Time
	GeneratedAt time.Time
}

// HistoryPort is the authoritative forecast_history store (Postgres).
// SaveBatch is the batch-loop write; GetLatestBatch is the
// GetLatestPrediction fallback; Query is QueryForecastHistory.
type HistoryPort interface {
	SaveBatch(ctx context.Context, batch []model.Prediction) error
	GetLatestBatch(ctx context.Context, tenantID, cuCode, metricName string) ([]model.Prediction, error)
	Query(ctx context.Context, q HistoryQuery) ([]model.Prediction, error)
}
