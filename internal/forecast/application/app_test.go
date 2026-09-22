package application

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
)

type nopTelemetry struct{}

func (nopTelemetry) QueryAggregation(context.Context, string, string, string, time.Time, time.Time, int64) ([]port.AggregatedPoint, error) {
	return nil, nil
}

type nopHistory struct{}

func (nopHistory) SaveBatch(context.Context, []model.Prediction) error { return nil }
func (nopHistory) GetLatestBatch(context.Context, string, string, string) ([]model.Prediction, error) {
	return nil, nil
}
func (nopHistory) Query(context.Context, port.HistoryQuery) ([]model.Prediction, error) {
	return nil, nil
}

type nopCache struct{}

func (nopCache) GetLatestBatch(context.Context, string, string, string) ([]model.Prediction, error) {
	return nil, nil
}
func (nopCache) SetLatestBatch(context.Context, []model.Prediction) error { return nil }

func TestNewApplication_WiresForecastLoop(t *testing.T) {
	app := NewApplication(Dependencies{
		Telemetry:     nopTelemetry{},
		History:       nopHistory{},
		Cache:         nopCache{},
		CycleInterval: 90 * time.Second,
		Targets: []model.ForecastTarget{{
			Enabled: true, TenantID: "t", CUCode: "cu", MetricName: "kw",
			Algorithm: model.AlgorithmMovingAverage, MovingAverageWindow: 4,
		}},
	})
	if app.ForecastLoop == nil {
		t.Fatal("expected ForecastLoop")
	}
	if app.Commands.RunForecastCycle == nil {
		t.Fatal("expected RunForecastCycle handler")
	}
	if app.Queries.GetLatestPrediction == nil || app.Queries.QueryForecastHistory == nil {
		t.Fatal("expected query handlers")
	}
}

func TestNewApplication_PanicsWithoutTelemetry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewApplication(Dependencies{History: nopHistory{}, Cache: nopCache{}})
}

func TestNewApplication_PanicsWithoutHistory(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewApplication(Dependencies{Telemetry: nopTelemetry{}, Cache: nopCache{}})
}

func TestNewApplication_PanicsWithoutCache(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewApplication(Dependencies{Telemetry: nopTelemetry{}, History: nopHistory{}})
}
