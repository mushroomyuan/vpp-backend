package service

import (
	"context"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

const day = 24 * time.Hour

// SamePeriodPriorPredictor predicts each future timestamp as the
// arithmetic mean of historical values at the same clock time on prior
// days (targetTimestamp − k×24h, k ≥ 1). v1 does not exclude event days
// (no Market calendar yet) and does not use the median.
//
// The caller must already bound history to ForecastTarget.SamePeriodLookbackDays
// (design plan §6). Missing days are skipped; at least one match per
// target timestamp is required.
type SamePeriodPriorPredictor struct{}

func (SamePeriodPriorPredictor) AlgorithmVersion() string {
	return string(model.AlgorithmSamePeriodPrior)
}

func (SamePeriodPriorPredictor) Predict(ctx context.Context, history []model.HistoryPoint, targetTimestamps []time.Time) ([]model.PredictedPoint, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(history) == 0 {
		return nil, fmt.Errorf("forecast: same_period_prior needs at least one history point")
	}
	if len(targetTimestamps) == 0 {
		return nil, fmt.Errorf("forecast: same_period_prior needs at least one target timestamp")
	}

	byUnix := make(map[int64]float64, len(history))
	oldest := history[0].Timestamp
	for _, h := range history {
		byUnix[h.Timestamp.Unix()] = h.Value
		if h.Timestamp.Before(oldest) {
			oldest = h.Timestamp
		}
	}

	out := make([]model.PredictedPoint, 0, len(targetTimestamps))
	for _, target := range targetTimestamps {
		mean, ok := samePeriodMean(target, oldest, byUnix)
		if !ok {
			return nil, fmt.Errorf("forecast: same_period_prior has no history at the same clock time as %s", target.UTC().Format(time.RFC3339))
		}
		out = append(out, model.PredictedPoint{TargetTimestamp: target, Value: mean})
	}
	return out, nil
}

func samePeriodMean(target, oldest time.Time, byUnix map[int64]float64) (float64, bool) {
	var sum float64
	var n int
	for k := 1; ; k++ {
		prior := target.Add(-time.Duration(k) * day)
		if prior.Before(oldest) {
			break
		}
		v, ok := byUnix[prior.Unix()]
		if !ok {
			continue
		}
		sum += v
		n++
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}
