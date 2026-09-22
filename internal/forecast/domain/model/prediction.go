package model

import "time"

// Prediction is one bi-temporal forecast value. GeneratedAt is when the
// batch ran; TargetTimestamp is the future instant being predicted.
// One batch shares a single GeneratedAt across horizon-steps points.
//
// This is the shape Redis caches (a whole batch), Postgres stores
// (forecast_history), and GetLatestPrediction / QueryForecastHistory
// return. SelectNextPoint picks among a []Prediction.
type Prediction struct {
	TenantID         string
	CUCode           string
	MetricName       string
	GeneratedAt      time.Time
	TargetTimestamp  time.Time
	PredictedValue   float64
	AlgorithmVersion string
}
