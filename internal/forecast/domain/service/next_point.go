package service

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

// SelectNextPoint picks the smallest target_timestamp >= now from batch
// ("from now, the next still-future point"). Redis cache hits and
// Postgres GetLatestBatch fallback both call this so the same now cannot
// yield two different answers depending on which store served the batch
// (design plan §6.1).
//
// ok is false when batch is empty or every point already lies in the
// past — the GetLatestPrediction use case then tries the other store, or
// returns NOT_FOUND if both miss.
func SelectNextPoint(batch []model.Prediction, now time.Time) (model.Prediction, bool) {
	var best model.Prediction
	found := false
	for _, p := range batch {
		if p.TargetTimestamp.Before(now) {
			continue
		}
		if !found || p.TargetTimestamp.Before(best.TargetTimestamp) {
			best = p
			found = true
		}
	}
	return best, found
}
