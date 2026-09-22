package service

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func TestSelectNextPoint_PicksSmallestTimestampNotBeforeNow(t *testing.T) {
	now := ts("2026-09-16T10:16:00Z")
	batch := []model.Prediction{
		pred("2026-09-16T10:45:00Z", 3),
		pred("2026-09-16T10:15:00Z", 1), // already past
		pred("2026-09-16T10:30:00Z", 2),
	}
	got, ok := SelectNextPoint(batch, now)
	if !ok {
		t.Fatal("expected a point")
	}
	if !got.TargetTimestamp.Equal(ts("2026-09-16T10:30:00Z")) {
		t.Errorf("got %s, want 10:30 (the 10:07-batch / 10:16-read case)", got.TargetTimestamp)
	}
	if got.PredictedValue != 2 {
		t.Errorf("value = %v, want 2", got.PredictedValue)
	}
}

func TestSelectNextPoint_IncludesTimestampEqualToNow(t *testing.T) {
	now := ts("2026-09-16T10:15:00Z")
	got, ok := SelectNextPoint([]model.Prediction{pred("2026-09-16T10:15:00Z", 9)}, now)
	if !ok || got.PredictedValue != 9 {
		t.Fatalf("equal-to-now must count as still-future, got ok=%v %+v", ok, got)
	}
}

func TestSelectNextPoint_EmptyOrAllPastIsMiss(t *testing.T) {
	now := ts("2026-09-16T11:01:00Z")
	if _, ok := SelectNextPoint(nil, now); ok {
		t.Fatal("nil batch must miss")
	}
	past := []model.Prediction{
		pred("2026-09-16T10:15:00Z", 1),
		pred("2026-09-16T10:30:00Z", 1),
		pred("2026-09-16T10:45:00Z", 1),
		pred("2026-09-16T11:00:00Z", 1),
	}
	if _, ok := SelectNextPoint(past, now); ok {
		t.Fatal("all-past batch must miss — GetLatestPrediction then falls back or returns NOT_FOUND")
	}
}

func TestSelectNextPoint_StaleRedisBatchFallsThroughToNewerPostgresBatch(t *testing.T) {
	// Redis still holds the 10:00 batch after 10:15 has elapsed (write-time
	// first step is now past). Postgres has a newer 10:15 batch that Redis
	// never got (write failed). The use case runs SelectNextPoint twice.
	now := ts("2026-09-16T10:16:00Z")
	redisBatch := []model.Prediction{
		pred("2026-09-16T10:15:00Z", 100),
		pred("2026-09-16T10:30:00Z", 100),
		pred("2026-09-16T10:45:00Z", 100),
		pred("2026-09-16T11:00:00Z", 100),
	}
	got, ok := SelectNextPoint(redisBatch, now)
	if !ok || !got.TargetTimestamp.Equal(ts("2026-09-16T10:30:00Z")) {
		t.Fatalf("full redis batch at 10:16 should still serve 10:30, got ok=%v %s", ok, got.TargetTimestamp)
	}

	staleFirstStepOnly := []model.Prediction{
		pred("2026-09-16T10:15:00Z", 100),
	}
	if _, ok := SelectNextPoint(staleFirstStepOnly, now); ok {
		t.Fatal("a cache that only stored write-time step 1 must miss at 10:16")
	}

	postgresNewerBatch := []model.Prediction{
		pred("2026-09-16T10:30:00Z", 80),
		pred("2026-09-16T10:45:00Z", 80),
		pred("2026-09-16T11:00:00Z", 80),
		pred("2026-09-16T11:15:00Z", 80),
	}
	got, ok = SelectNextPoint(postgresNewerBatch, now)
	if !ok {
		t.Fatal("postgres newer batch must hit")
	}
	if !got.TargetTimestamp.Equal(ts("2026-09-16T10:30:00Z")) || got.PredictedValue != 80 {
		t.Errorf("fallback = %+v, want 10:30 from the newer batch", got)
	}
}

func pred(rfc3339 string, value float64) model.Prediction {
	return model.Prediction{
		TenantID:        "t",
		CUCode:          "cu",
		MetricName:      "kw",
		GeneratedAt:     ts("2026-09-16T10:00:00Z"),
		TargetTimestamp: ts(rfc3339),
		PredictedValue:  value,
	}
}
