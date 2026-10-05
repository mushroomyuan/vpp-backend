// Package forecaststub is the ForecastProvider used until a Planner opts into Forecast.
// The SOC path does not call it, and this process does not dial the Forecast service.
package forecaststub

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// ErrNotImplemented is returned by every call.
var ErrNotImplemented = errors.New("forecast_stub: forecast is not connected")

// Stub always fails. There is nothing to configure.
type Stub struct{}

func New() *Stub { return &Stub{} }

var _ port.ForecastProvider = (*Stub)(nil)

// Latest returns ErrNotImplemented.
func (s *Stub) Latest(_ context.Context, _ port.ForecastQuery) (port.Forecast, error) {
	return port.Forecast{}, ErrNotImplemented
}
