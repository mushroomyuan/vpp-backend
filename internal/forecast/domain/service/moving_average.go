package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

// MovingAveragePredictor fills every future timestamp with the arithmetic
// mean of the history it is given. v1 does not extrapolate a trend.
//
// The caller must already trim history to ForecastTarget.MovingAverageWindow
// most-recent buckets (design plan §6); this type is a singleton in the
// Registry and has no per-target window of its own.
type MovingAveragePredictor struct{}

func (MovingAveragePredictor) AlgorithmVersion() string {
	return string(model.AlgorithmMovingAverage)
}

func (MovingAveragePredictor) Predict(ctx context.Context, history []model.HistoryPoint, targetTimestamps []time.Time) ([]model.PredictedPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("forecast: moving_average needs at least one history point")
	}
	if len(targetTimestamps) == 0 {
		return nil, fmt.Errorf("forecast: moving_average needs at least one target timestamp")
	}

	var sum float64
	for _, h := range history {
		sum += h.Value
	}
	mean := sum / float64(len(history))

	out := make([]model.PredictedPoint, len(targetTimestamps))
	for i, ts := range targetTimestamps {
		out[i] = model.PredictedPoint{TargetTimestamp: ts, Value: mean}
	}
	return out, nil
}
