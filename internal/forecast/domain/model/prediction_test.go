package model

import (
	"testing"
	"time"
)

func TestPrediction_BiTemporal(t *testing.T) {
	generated := time.Date(2026, 9, 16, 10, 7, 0, 0, time.UTC)
	target := time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC)
	p := Prediction{
		TenantID:         "tenant-a",
		CUCode:           "cu-battery-1",
		MetricName:       "active_power_kw",
		GeneratedAt:      generated,
		TargetTimestamp:  target,
		PredictedValue:   100,
		AlgorithmVersion: "moving_average",
	}
	if !p.GeneratedAt.Equal(generated) {
		t.Errorf("GeneratedAt = %s, want %s", p.GeneratedAt, generated)
	}
	if !p.TargetTimestamp.Equal(target) {
		t.Errorf("TargetTimestamp = %s, want %s", p.TargetTimestamp, target)
	}
	if p.PredictedValue != 100 || p.AlgorithmVersion != "moving_average" {
		t.Errorf("value/version: %+v", p)
	}
}

func TestHistoryPointAndPredictedPoint(t *testing.T) {
	ts := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	h := HistoryPoint{Timestamp: ts, Value: 42}
	if !h.Timestamp.Equal(ts) || h.Value != 42 {
		t.Errorf("HistoryPoint: %+v", h)
	}
	out := PredictedPoint{TargetTimestamp: ts.Add(15 * time.Minute), Value: 40}
	if !out.TargetTimestamp.Equal(ts.Add(15*time.Minute)) || out.Value != 40 {
		t.Errorf("PredictedPoint: %+v", out)
	}
}
