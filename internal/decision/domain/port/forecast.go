package port

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// ForecastQuery asks for one predicted metric on one CU.
type ForecastQuery struct {
	TenantID string
	CUCode   string
	MetricID contracts.MetricID
}

// Forecast is a single predicted value. The SOC path does not read it.
type Forecast struct {
	MetricID    contracts.MetricID
	Value       float64
	GeneratedAt time.Time
	TargetAt    time.Time
}

// ForecastProvider is the optional prediction source for a future Planner.
// The only implementation returns an error. This process does not call it.
type ForecastProvider interface {
	Latest(ctx context.Context, query ForecastQuery) (Forecast, error)
}
