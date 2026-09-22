package domain

import "errors"

// ErrPredictionNotFound is returned by GetLatestPrediction when this
// target has never been forecast, or every point in the latest batch
// already lies in the past (design plan §3 / §6.1).
var ErrPredictionNotFound = errors.New("prediction not found")
