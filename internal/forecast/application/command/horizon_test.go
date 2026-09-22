package command

import (
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
)

func TestAlignedFutureBuckets_NextBoundaryStrictlyAfterNow(t *testing.T) {
	now := time.Date(2026, 9, 16, 10, 7, 0, 0, time.UTC)
	got := alignedFutureBuckets(now, 900, 4)
	want := []time.Time{
		time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC),
		time.Date(2026, 9, 16, 10, 30, 0, 0, time.UTC),
		time.Date(2026, 9, 16, 10, 45, 0, 0, time.UTC),
		time.Date(2026, 9, 16, 11, 0, 0, 0, time.UTC),
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Errorf("bucket[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestAlignedFutureBuckets_ExactBoundaryIsSkipped(t *testing.T) {
	now := time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC)
	got := alignedFutureBuckets(now, 900, 1)
	if len(got) != 1 || !got[0].Equal(time.Date(2026, 9, 16, 10, 30, 0, 0, time.UTC)) {
		t.Fatalf("got %v, want 10:30 (strictly after 10:15)", got)
	}
}

func TestAlignedFutureBuckets_InvalidArgs(t *testing.T) {
	if alignedFutureBuckets(time.Now(), 0, 4) != nil {
		t.Fatal("stepSeconds <= 0 must return nil")
	}
	if alignedFutureBuckets(time.Now(), 900, 0) != nil {
		t.Fatal("horizonSteps < 1 must return nil")
	}
}

func TestHistoryFromAggregation_MovingAverageTrimsToWindowAndSkipsNilAvg(t *testing.T) {
	target := model.ForecastTarget{
		Algorithm:           model.AlgorithmMovingAverage,
		MovingAverageWindow: 2,
	}
	now := ts("2026-09-16T10:07:00Z")
	points := []port.AggregatedPoint{
		{Timestamp: ts("2026-09-16T09:15:00Z"), Avg: f64(1), Last: f64(100)},
		{Timestamp: ts("2026-09-16T09:30:00Z"), Last: f64(100)}, // empty Avg
		{Timestamp: ts("2026-09-16T09:45:00Z"), Avg: f64(3), Last: f64(100)},
		{Timestamp: ts("2026-09-16T10:00:00Z"), Avg: f64(5), Last: f64(100)},
	}
	got := historyFromAggregation(target, points, now)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (window of last Avg points)", len(got))
	}
	if got[0].Value != 3 || got[1].Value != 5 {
		t.Errorf("values = %v, %v, want 3, 5", got[0].Value, got[1].Value)
	}
}

func TestHistoryFromAggregation_SamePeriodPriorUsesLastAndLookback(t *testing.T) {
	target := model.ForecastTarget{
		Algorithm:              model.AlgorithmSamePeriodPrior,
		SamePeriodLookbackDays: 2,
	}
	now := ts("2026-09-16T10:07:00Z")
	points := []port.AggregatedPoint{
		{Timestamp: ts("2026-09-13T10:15:00Z"), Avg: f64(999), Last: f64(1)},
		{Timestamp: ts("2026-09-14T10:15:00Z"), Avg: f64(999), Last: f64(10)},
		{Timestamp: ts("2026-09-15T10:15:00Z"), Avg: f64(999)}, // empty Last
		{Timestamp: ts("2026-09-15T10:00:00Z"), Avg: f64(999), Last: f64(20)},
	}
	got := historyFromAggregation(target, points, now)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (lookback Last points)", len(got))
	}
	if got[0].Value != 10 || got[1].Value != 20 {
		t.Errorf("values = %v, %v, want Last 10 then 20", got[0].Value, got[1].Value)
	}
}
