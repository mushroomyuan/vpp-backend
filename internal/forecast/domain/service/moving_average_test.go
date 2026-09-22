package service

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
)

func TestMovingAverage_FillsEveryHorizonPointWithMean(t *testing.T) {
	p := MovingAveragePredictor{}
	history := []model.HistoryPoint{
		{Timestamp: ts("2026-09-16T09:30:00Z"), Value: 10},
		{Timestamp: ts("2026-09-16T09:45:00Z"), Value: 20},
		{Timestamp: ts("2026-09-16T10:00:00Z"), Value: 30},
	}
	targets := []time.Time{ts("2026-09-16T10:15:00Z"), ts("2026-09-16T10:30:00Z")}

	got, err := p.Predict(context.Background(), history, targets)
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	const mean = 20.0
	for i, pt := range got {
		if !pt.TargetTimestamp.Equal(targets[i]) {
			t.Errorf("point[%d] timestamp = %s, want %s", i, pt.TargetTimestamp, targets[i])
		}
		if pt.Value != mean {
			t.Errorf("point[%d] value = %v, want %v", i, pt.Value, mean)
		}
	}
}

func TestMovingAverage_EmptyHistoryErrors(t *testing.T) {
	_, err := MovingAveragePredictor{}.Predict(context.Background(), nil, []time.Time{ts("2026-09-16T10:15:00Z")})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMovingAverage_EmptyTargetsError(t *testing.T) {
	_, err := MovingAveragePredictor{}.Predict(context.Background(), []model.HistoryPoint{{Value: 1}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMovingAverage_HonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := MovingAveragePredictor{}.Predict(ctx, []model.HistoryPoint{{Value: 1}}, []time.Time{ts("2026-09-16T10:15:00Z")})
	if err == nil {
		t.Fatal("expected ctx error")
	}
}

func TestMovingAverage_AlgorithmVersion(t *testing.T) {
	if got := (MovingAveragePredictor{}).AlgorithmVersion(); got != string(model.AlgorithmMovingAverage) {
		t.Errorf("AlgorithmVersion = %q", got)
	}
}
