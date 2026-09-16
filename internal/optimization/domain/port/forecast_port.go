package port

import (
	"context"
	"time"
)

// Prediction is a single forecast value for one CU/metric at one target
// time. Field names mirror the bi-temporal shape discussion/2026-09-04.md
// §3.2 designed for forecast_history (generated_at + target_timestamp),
// trimmed to what a v1 consumer would need to read one "latest" value.
type Prediction struct {
	CUCode           string
	MetricName       string
	GeneratedAt      time.Time
	TargetTimestamp  time.Time
	PredictedValue   float64
	AlgorithmVersion string
}

// ForecastPort is what a forecast-dependent rule would call to read the
// latest prediction for a CU/metric. Optimization v1's rule engine
// (domain/service.Evaluator) does not call this at all — see the
// Optimization design plan §7: the Forecast service itself does not exist
// yet, and v1's SOCThresholdRule is deliberately scoped to need only
// current Telemetry state.
//
// This interface exists now, ahead of any real caller, so that:
//   - the shape is already right on the day Forecast (a separate design
//     session) lands, and
//   - a future forecast-dependent rule type can be added to Rules without
//     a breaking change to this port.
//
// The only implementation today (adapter/outbound/forecast_stub) always
// returns ErrNotImplemented.
type ForecastPort interface {
	GetLatestPrediction(ctx context.Context, tenantID, cuCode, metricName string) (*Prediction, error)
}
