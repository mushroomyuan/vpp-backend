package model

import "time"

// HistoryPoint is one downsampled telemetry bucket fed into a Predictor.
// Timestamp is the bucket's window start (aligned to step-seconds).
// Value is already the aggregation the algorithm asked for (Avg for
// moving_average, Last for same_period_prior).
type HistoryPoint struct {
	Timestamp time.Time
	Value     float64
}

// PredictedPoint is one Predictor output: a value at a future
// target timestamp, before identity / algorithm version are attached
// to make a Prediction.
type PredictedPoint struct {
	TargetTimestamp time.Time
	Value           float64
}
