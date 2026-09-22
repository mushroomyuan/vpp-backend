package service

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func TestSamePeriodPrior_AveragesMatchingClockTimes(t *testing.T) {
	p := SamePeriodPriorPredictor{}
	// Two prior days at 10:15, one unrelated 10:00 that must be ignored.
	history := []model.HistoryPoint{
		{Timestamp: ts("2026-09-14T10:15:00Z"), Value: 10},
		{Timestamp: ts("2026-09-15T10:15:00Z"), Value: 20},
		{Timestamp: ts("2026-09-15T10:00:00Z"), Value: 999},
		{Timestamp: ts("2026-09-14T10:30:00Z"), Value: 40},
		{Timestamp: ts("2026-09-15T10:30:00Z"), Value: 50},
	}
	targets := []time.Time{ts("2026-09-16T10:15:00Z"), ts("2026-09-16T10:30:00Z")}

	got, err := p.Predict(context.Background(), history, targets)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Value != 15 {
		t.Errorf("10:15 mean = %v, want 15", got[0].Value)
	}
	if got[1].Value != 45 {
		t.Errorf("10:30 mean = %v, want 45", got[1].Value)
	}
}

func TestSamePeriodPrior_SkipsMissingDays(t *testing.T) {
	p := SamePeriodPriorPredictor{}
	history := []model.HistoryPoint{
		{Timestamp: ts("2026-09-13T10:15:00Z"), Value: 10},
		// 09-14 missing
		{Timestamp: ts("2026-09-15T10:15:00Z"), Value: 30},
	}
	got, err := p.Predict(context.Background(), history, []time.Time{ts("2026-09-16T10:15:00Z")})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if got[0].Value != 20 {
		t.Errorf("mean of available days = %v, want 20", got[0].Value)
	}
}

func TestSamePeriodPrior_NoMatchErrors(t *testing.T) {
	history := []model.HistoryPoint{
		{Timestamp: ts("2026-09-15T09:00:00Z"), Value: 1},
	}
	_, err := SamePeriodPriorPredictor{}.Predict(context.Background(), history, []time.Time{ts("2026-09-16T10:15:00Z")})
	if err == nil {
		t.Fatal("expected error when no same-clock-time history exists")
	}
}

func TestSamePeriodPrior_DoesNotUseTheTargetInstantItself(t *testing.T) {
	history := []model.HistoryPoint{
		{Timestamp: ts("2026-09-16T10:15:00Z"), Value: 100}, // same instant as the target — not a prior day
	}
	_, err := SamePeriodPriorPredictor{}.Predict(context.Background(), history, []time.Time{ts("2026-09-16T10:15:00Z")})
	if err == nil {
		t.Fatal("k starts at 1; the target instant must not count as history")
	}
}

func TestSamePeriodPrior_AlgorithmVersion(t *testing.T) {
	if got := (SamePeriodPriorPredictor{}).AlgorithmVersion(); got != string(model.AlgorithmSamePeriodPrior) {
		t.Errorf("AlgorithmVersion = %q", got)
	}
}
