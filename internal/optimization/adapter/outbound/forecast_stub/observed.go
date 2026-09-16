package forecaststub

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

// Observed wraps a ForecastPort and records every GetLatestPrediction call
// on Observer. v1's rule engine does not call Forecast; this wrapper exists
// so the metric series is live the day a forecast-dependent rule lands.
type Observed struct {
	inner port.ForecastPort
	obs   port.Observer
}

var _ port.ForecastPort = (*Observed)(nil)

// NewObserved returns inner unchanged when obs is nil.
func NewObserved(inner port.ForecastPort, obs port.Observer) port.ForecastPort {
	if inner == nil {
		return nil
	}
	if obs == nil {
		return inner
	}
	return &Observed{inner: inner, obs: obs}
}

func (o *Observed) GetLatestPrediction(ctx context.Context, tenantID, cuCode, metricName string) (*port.Prediction, error) {
	pred, err := o.inner.GetLatestPrediction(ctx, tenantID, cuCode, metricName)
	o.obs.ObserveForecast(forecastResult(err))
	return pred, err
}

func forecastResult(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, ErrNotImplemented):
		return "not_implemented"
	default:
		return "error"
	}
}
