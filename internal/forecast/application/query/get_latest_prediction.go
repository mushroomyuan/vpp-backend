package query

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/forecast/domain"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
)

// GetLatestPrediction is "from now, the next still-future predicted
// point" for one CU metric. Now zero means time.Now(); the gRPC handler
// leaves it zero so the answer moves as wall-clock time advances.
type GetLatestPrediction struct {
	TenantID   string
	CUCode     string
	MetricName string
	Now        time.Time
}

type GetLatestPredictionHandler = decorator.QueryHandler[GetLatestPrediction, model.Prediction]

type getLatestPredictionHandler struct {
	cache   port.CachePort
	history port.HistoryPort
}

// NewGetLatestPredictionHandler builds the query. cache and history are
// both required: a Redis miss (or an all-past cached batch) falls back
// to Postgres GetLatestBatch, then SelectNextPoint runs on that batch
// too (design plan §6.1).
func NewGetLatestPredictionHandler(
	cache port.CachePort,
	history port.HistoryPort,
	metricsClient decorator.MetricsClient,
) GetLatestPredictionHandler {
	if cache == nil {
		panic("NewGetLatestPredictionHandler: cache is required")
	}
	if history == nil {
		panic("NewGetLatestPredictionHandler: history is required")
	}
	if metricsClient == nil {
		metricsClient = nopMetrics{}
	}
	return decorator.ApplyQueryDecorators[GetLatestPrediction, model.Prediction](
		getLatestPredictionHandler{cache: cache, history: history},
		metricsClient,
	)
}

func (h getLatestPredictionHandler) Handle(ctx context.Context, q GetLatestPrediction) (model.Prediction, error) {
	if err := requireIdentity(q.TenantID, q.CUCode, q.MetricName); err != nil {
		return model.Prediction{}, err
	}
	now := q.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()

	cached, cacheErr := h.cache.GetLatestBatch(ctx, q.TenantID, q.CUCode, q.MetricName)
	if cacheErr != nil {
		logging.Warnf(ctx, logrus.Fields{
			"component":   "GetLatestPrediction",
			"tenant_id":   q.TenantID,
			"cu_code":     q.CUCode,
			"metric_name": q.MetricName,
			"error":       cacheErr.Error(),
		}, "cache GetLatestBatch failed, falling back to Postgres")
	} else if p, ok := service.SelectNextPoint(cached, now); ok {
		return p, nil
	}

	batch, err := h.history.GetLatestBatch(ctx, q.TenantID, q.CUCode, q.MetricName)
	if err != nil {
		return model.Prediction{}, fmt.Errorf("forecast: GetLatestPrediction: %w", err)
	}
	if p, ok := service.SelectNextPoint(batch, now); ok {
		return p, nil
	}
	return model.Prediction{}, fmt.Errorf("%w: %s/%s/%s", domain.ErrPredictionNotFound, q.TenantID, q.CUCode, q.MetricName)
}
