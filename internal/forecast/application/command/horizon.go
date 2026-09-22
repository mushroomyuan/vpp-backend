package command

import (
	"sort"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
)

const day = 24 * time.Hour

// alignedFutureBuckets returns horizonSteps timestamps: the first is the
// next step-seconds clock-bucket boundary strictly after now (e.g. 900s
// → :00/:15/:30/:45), then every step after that. Alignment is Unix-epoch
// so it matches Telemetry's time_bucket and same_period_prior's −N×24h
// lookup (design plan §4).
func alignedFutureBuckets(now time.Time, stepSeconds int64, horizonSteps int) []time.Time {
	if stepSeconds <= 0 || horizonSteps < 1 {
		return nil
	}
	next := ((now.Unix() / stepSeconds) + 1) * stepSeconds
	out := make([]time.Time, horizonSteps)
	for i := 0; i < horizonSteps; i++ {
		out[i] = time.Unix(next+int64(i)*stepSeconds, 0).UTC()
	}
	return out
}

// historyFromAggregation maps QueryAggregation buckets into the series
// one Predictor expects. Window / lookback trimming lives here so the
// Registry predictors stay pure functions of the points they receive.
func historyFromAggregation(target model.ForecastTarget, points []port.AggregatedPoint, now time.Time) []model.HistoryPoint {
	switch target.Algorithm {
	case model.AlgorithmMovingAverage:
		return trimMovingAverage(points, target.MovingAverageWindow)
	case model.AlgorithmSamePeriodPrior:
		return trimSamePeriodPrior(points, now, target.SamePeriodLookbackDays)
	default:
		return nil
	}
}

func trimMovingAverage(points []port.AggregatedPoint, window int) []model.HistoryPoint {
	out := make([]model.HistoryPoint, 0, len(points))
	for _, p := range points {
		if p.Avg == nil {
			continue
		}
		out = append(out, model.HistoryPoint{Timestamp: p.Timestamp.UTC(), Value: *p.Avg})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	if window > 0 && len(out) > window {
		out = out[len(out)-window:]
	}
	return out
}

func trimSamePeriodPrior(points []port.AggregatedPoint, now time.Time, lookbackDays int) []model.HistoryPoint {
	cutoff := now.UTC().Add(-time.Duration(lookbackDays) * day)
	out := make([]model.HistoryPoint, 0, len(points))
	for _, p := range points {
		if p.Last == nil {
			continue
		}
		ts := p.Timestamp.UTC()
		if ts.Before(cutoff) {
			continue
		}
		out = append(out, model.HistoryPoint{Timestamp: ts, Value: *p.Last})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out
}
