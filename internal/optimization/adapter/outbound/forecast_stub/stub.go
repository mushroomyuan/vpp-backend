// Package forecaststub is the v1 placeholder implementation of
// domain/port.ForecastPort. See the Optimization design plan §7 and
// discussion/2026-09-04.md's forecast scope decision: the Forecast
// service does not exist yet (its design is deliberately left to a
// separate session), and v1's rule engine does not call this at all.
//
// This exists so anything that holds a port.ForecastPort dependency has
// something to construct today, without pretending Forecast is real.
package forecaststub

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
)

// ErrNotImplemented is returned by every call — Forecast is not built yet.
var ErrNotImplemented = errors.New("forecast_stub: Forecast service is not implemented yet")

// Stub is the always-fails ForecastPort implementation.
type Stub struct{}

// New returns a Stub. There is nothing to configure — it always fails.
func New() *Stub { return &Stub{} }

var _ port.ForecastPort = (*Stub)(nil)

// GetLatestPrediction always returns ErrNotImplemented.
func (s *Stub) GetLatestPrediction(_ context.Context, _, _, _ string) (*port.Prediction, error) {
	return nil, ErrNotImplemented
}
