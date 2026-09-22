// Package model holds Forecast's domain types: the bi-temporal Prediction
// that Redis/Postgres/gRPC all share, the Predictor input/output points,
// and the flat ForecastTarget configuration (design plan §5).
package model

// AlgorithmID identifies which Predictor implementation a ForecastTarget
// is bound to. It is the Registry key; AlgorithmVersion() on a Predictor
// is the longer provenance string written to forecast_history.
type AlgorithmID string

const (
	AlgorithmMovingAverage   AlgorithmID = "moving_average"
	AlgorithmSamePeriodPrior AlgorithmID = "same_period_prior"
)

// Known reports whether id is one of v1's two naive algorithms.
func (id AlgorithmID) Known() bool {
	switch id {
	case AlgorithmMovingAverage, AlgorithmSamePeriodPrior:
		return true
	default:
		return false
	}
}
