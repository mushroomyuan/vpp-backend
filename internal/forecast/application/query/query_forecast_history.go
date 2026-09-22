package query

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// QueryForecastHistory returns persisted points whose target_timestamp
// falls in [StartTime, EndTime]. GeneratedAt zero = latest generation
// per timestamp; non-zero = only that batch.
type QueryForecastHistory struct {
	TenantID    string
	CUCode      string
	MetricName  string
	StartTime   time.Time
	EndTime     time.Time
	GeneratedAt time.Time
}

type QueryForecastHistoryHandler = decorator.QueryHandler[QueryForecastHistory, []model.Prediction]

type queryForecastHistoryHandler struct {
	history port.HistoryPort
}

func NewQueryForecastHistoryHandler(
	history port.HistoryPort,
	metricsClient decorator.MetricsClient,
) QueryForecastHistoryHandler {
	if history == nil {
		panic("NewQueryForecastHistoryHandler: history is required")
	}
	if metricsClient == nil {
		metricsClient = nopMetrics{}
	}
	return decorator.ApplyQueryDecorators[QueryForecastHistory, []model.Prediction](
		queryForecastHistoryHandler{history: history},
		metricsClient,
	)
}

func (h queryForecastHistoryHandler) Handle(ctx context.Context, q QueryForecastHistory) ([]model.Prediction, error) {
	if err := requireIdentity(q.TenantID, q.CUCode, q.MetricName); err != nil {
		return nil, err
	}
	if q.StartTime.IsZero() || q.EndTime.IsZero() {
		return nil, fmt.Errorf("forecast: start_time and end_time are required")
	}
	if q.StartTime.After(q.EndTime) {
		return nil, fmt.Errorf("forecast: start_time cannot be after end_time")
	}
	out, err := h.history.Query(ctx, port.HistoryQuery{
		TenantID:    q.TenantID,
		CUCode:      q.CUCode,
		MetricName:  q.MetricName,
		StartTime:   q.StartTime.UTC(),
		EndTime:     q.EndTime.UTC(),
		GeneratedAt: q.GeneratedAt.UTC(),
	})
	if err != nil {
		return nil, fmt.Errorf("forecast: QueryForecastHistory: %w", err)
	}
	if out == nil {
		out = []model.Prediction{}
	}
	return out, nil
}

func requireIdentity(tenantID, cuCode, metricName string) error {
	if tenantID == "" || cuCode == "" || metricName == "" {
		return fmt.Errorf("forecast: tenant_id, cu_code, and metric_name are required")
	}
	return nil
}

type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }
