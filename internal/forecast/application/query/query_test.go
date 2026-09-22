package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
)

type stubCache struct {
	batch []model.Prediction
	err   error
	calls int
}

func (s *stubCache) GetLatestBatch(context.Context, string, string, string) ([]model.Prediction, error) {
	s.calls++
	return s.batch, s.err
}

func (s *stubCache) SetLatestBatch(context.Context, []model.Prediction) error { return nil }

type stubHistory struct {
	latest      []model.Prediction
	latestErr   error
	latestCalls int
	query       port.HistoryQuery
	queryOut    []model.Prediction
	queryErr    error
}

func (s *stubHistory) SaveBatch(context.Context, []model.Prediction) error { return nil }

func (s *stubHistory) GetLatestBatch(context.Context, string, string, string) ([]model.Prediction, error) {
	s.latestCalls++
	return s.latest, s.latestErr
}

func (s *stubHistory) Query(_ context.Context, q port.HistoryQuery) ([]model.Prediction, error) {
	s.query = q
	return s.queryOut, s.queryErr
}

func ts(rfc3339 string) time.Time {
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		panic(err)
	}
	return t
}

func pred(targetRFC3339 string, value float64) model.Prediction {
	return model.Prediction{
		TenantID:         "tenant-a",
		CUCode:           "cu-battery-1",
		MetricName:       "active_power_kw",
		GeneratedAt:      ts("2026-09-16T10:07:00Z"),
		TargetTimestamp:  ts(targetRFC3339),
		PredictedValue:   value,
		AlgorithmVersion: "moving_average",
	}
}

func latestQuery(now string) GetLatestPrediction {
	return GetLatestPrediction{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		Now: ts(now),
	}
}

func TestGetLatestPrediction_CacheHitDoesNotTouchHistory(t *testing.T) {
	cache := &stubCache{batch: []model.Prediction{
		pred("2026-09-16T10:15:00Z", 1),
		pred("2026-09-16T10:30:00Z", 2),
	}}
	hist := &stubHistory{}
	h := NewGetLatestPredictionHandler(cache, hist, nil)

	got, err := h.Handle(context.Background(), latestQuery("2026-09-16T10:16:00Z"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.PredictedValue != 2 || !got.TargetTimestamp.Equal(ts("2026-09-16T10:30:00Z")) {
		t.Errorf("got %+v, want 10:30 value 2", got)
	}
	if hist.latestCalls != 0 {
		t.Fatal("Postgres must not be queried on a cache hit that still has a future point")
	}
}

func TestGetLatestPrediction_AllPastCacheFallsBackToNewerPostgresBatch(t *testing.T) {
	cache := &stubCache{batch: []model.Prediction{
		pred("2026-09-16T10:15:00Z", 100),
	}}
	hist := &stubHistory{latest: []model.Prediction{
		pred("2026-09-16T10:30:00Z", 80),
		pred("2026-09-16T10:45:00Z", 80),
	}}
	h := NewGetLatestPredictionHandler(cache, hist, nil)

	got, err := h.Handle(context.Background(), latestQuery("2026-09-16T10:16:00Z"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.PredictedValue != 80 || !got.TargetTimestamp.Equal(ts("2026-09-16T10:30:00Z")) {
		t.Errorf("fallback = %+v, want newer-batch 10:30", got)
	}
	if hist.latestCalls != 1 {
		t.Fatalf("expected Postgres fallback, calls=%d", hist.latestCalls)
	}
}

func TestGetLatestPrediction_CacheErrorFallsBackToPostgres(t *testing.T) {
	cache := &stubCache{err: errors.New("redis down")}
	hist := &stubHistory{latest: []model.Prediction{
		pred("2026-09-16T10:30:00Z", 7),
	}}
	h := NewGetLatestPredictionHandler(cache, hist, nil)

	got, err := h.Handle(context.Background(), latestQuery("2026-09-16T10:16:00Z"))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got.PredictedValue != 7 {
		t.Errorf("got %+v", got)
	}
}

func TestGetLatestPrediction_BothMissIsNotFound(t *testing.T) {
	h := NewGetLatestPredictionHandler(&stubCache{}, &stubHistory{}, nil)
	_, err := h.Handle(context.Background(), latestQuery("2026-09-16T10:16:00Z"))
	if !errors.Is(err, domain.ErrPredictionNotFound) {
		t.Fatalf("got %v, want ErrPredictionNotFound", err)
	}
}

func TestGetLatestPrediction_AllPastOnBothStoresIsNotFound(t *testing.T) {
	past := []model.Prediction{pred("2026-09-16T10:15:00Z", 1)}
	h := NewGetLatestPredictionHandler(&stubCache{batch: past}, &stubHistory{latest: past}, nil)
	_, err := h.Handle(context.Background(), latestQuery("2026-09-16T11:01:00Z"))
	if !errors.Is(err, domain.ErrPredictionNotFound) {
		t.Fatalf("got %v, want ErrPredictionNotFound", err)
	}
}

func TestGetLatestPrediction_RequiresIdentity(t *testing.T) {
	h := NewGetLatestPredictionHandler(&stubCache{}, &stubHistory{}, nil)
	_, err := h.Handle(context.Background(), GetLatestPrediction{CUCode: "cu", MetricName: "kw"})
	if err == nil {
		t.Fatal("expected identity error")
	}
}

func TestGetLatestPrediction_HistoryErrorSurfaces(t *testing.T) {
	h := NewGetLatestPredictionHandler(&stubCache{}, &stubHistory{latestErr: errors.New("db down")}, nil)
	_, err := h.Handle(context.Background(), latestQuery("2026-09-16T10:16:00Z"))
	if err == nil {
		t.Fatal("expected history error")
	}
}

func TestNewGetLatestPredictionHandler_PanicsWithoutCache(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewGetLatestPredictionHandler(nil, &stubHistory{}, nil)
}

func TestQueryForecastHistory_ForwardsWindowAndGeneratedAt(t *testing.T) {
	hist := &stubHistory{queryOut: []model.Prediction{pred("2026-09-16T10:15:00Z", 1)}}
	h := NewQueryForecastHistoryHandler(hist, nil)
	generated := ts("2026-09-16T06:07:00Z")
	got, err := h.Handle(context.Background(), QueryForecastHistory{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		StartTime: ts("2026-09-16T10:00:00Z"), EndTime: ts("2026-09-16T12:00:00Z"),
		GeneratedAt: generated,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if !hist.query.GeneratedAt.Equal(generated) {
		t.Errorf("GeneratedAt = %s", hist.query.GeneratedAt)
	}
	if !hist.query.StartTime.Equal(ts("2026-09-16T10:00:00Z")) || !hist.query.EndTime.Equal(ts("2026-09-16T12:00:00Z")) {
		t.Errorf("window = [%s, %s]", hist.query.StartTime, hist.query.EndTime)
	}
}

func TestQueryForecastHistory_UnsetGeneratedAtStaysZero(t *testing.T) {
	hist := &stubHistory{}
	h := NewQueryForecastHistoryHandler(hist, nil)
	_, err := h.Handle(context.Background(), QueryForecastHistory{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		StartTime: ts("2026-09-16T10:00:00Z"), EndTime: ts("2026-09-16T12:00:00Z"),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !hist.query.GeneratedAt.IsZero() {
		t.Errorf("GeneratedAt should stay zero, got %s", hist.query.GeneratedAt)
	}
}

func TestQueryForecastHistory_NilResultIsEmptySlice(t *testing.T) {
	h := NewQueryForecastHistoryHandler(&stubHistory{}, nil)
	got, err := h.Handle(context.Background(), QueryForecastHistory{
		TenantID: "t", CUCode: "cu", MetricName: "kw",
		StartTime: ts("2026-09-16T10:00:00Z"), EndTime: ts("2026-09-16T12:00:00Z"),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
}

func TestQueryForecastHistory_ValidatesWindow(t *testing.T) {
	h := NewQueryForecastHistoryHandler(&stubHistory{}, nil)
	if _, err := h.Handle(context.Background(), QueryForecastHistory{
		TenantID: "t", CUCode: "cu", MetricName: "kw",
	}); err == nil {
		t.Fatal("missing window must error")
	}
	start := ts("2026-09-16T12:00:00Z")
	if _, err := h.Handle(context.Background(), QueryForecastHistory{
		TenantID: "t", CUCode: "cu", MetricName: "kw",
		StartTime: start, EndTime: start.Add(-time.Hour),
	}); err == nil {
		t.Fatal("inverted window must error")
	}
}

func TestNewQueryForecastHistoryHandler_PanicsWithoutHistory(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewQueryForecastHistoryHandler(nil, nil)
}
