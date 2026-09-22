package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
)

type stubTelemetry struct {
	points map[string][]port.AggregatedPoint
	errs   map[string]error
	calls  []aggCall
}

type aggCall struct {
	tenantID, cuCode, metricName string
	start, end                   time.Time
	stepSeconds                  int64
}

func (s *stubTelemetry) QueryAggregation(_ context.Context, tenantID, cuCode, metricName string, start, end time.Time, stepSeconds int64) ([]port.AggregatedPoint, error) {
	s.calls = append(s.calls, aggCall{tenantID, cuCode, metricName, start, end, stepSeconds})
	if err := s.errs[cuCode]; err != nil {
		return nil, err
	}
	return s.points[cuCode], nil
}

type stubHistory struct {
	batches [][]model.Prediction
	err     error
}

func (s *stubHistory) SaveBatch(_ context.Context, batch []model.Prediction) error {
	if s.err != nil {
		return s.err
	}
	copied := make([]model.Prediction, len(batch))
	copy(copied, batch)
	s.batches = append(s.batches, copied)
	return nil
}

func (s *stubHistory) GetLatestBatch(context.Context, string, string, string) ([]model.Prediction, error) {
	return nil, nil
}

func (s *stubHistory) Query(context.Context, port.HistoryQuery) ([]model.Prediction, error) {
	return nil, nil
}

type stubCache struct {
	batches [][]model.Prediction
	err     error
}

func (s *stubCache) GetLatestBatch(context.Context, string, string, string) ([]model.Prediction, error) {
	return nil, nil
}

func (s *stubCache) SetLatestBatch(_ context.Context, batch []model.Prediction) error {
	if s.err != nil {
		return s.err
	}
	copied := make([]model.Prediction, len(batch))
	copy(copied, batch)
	s.batches = append(s.batches, copied)
	return nil
}

func newCycleHandler(tel *stubTelemetry, hist *stubHistory, cache *stubCache, targets ...model.ForecastTarget) RunForecastCycleHandler {
	if tel == nil {
		tel = &stubTelemetry{}
	}
	if hist == nil {
		hist = &stubHistory{}
	}
	if cache == nil {
		cache = &stubCache{}
	}
	return newRunForecastCycleHandler(tel, hist, cache, nil, targets, 4, 900, 168*time.Hour, nil, nil)
}

func avgHistory() []port.AggregatedPoint {
	return []port.AggregatedPoint{
		{Timestamp: ts("2026-09-16T09:15:00Z"), Avg: f64(10), Last: f64(10)},
		{Timestamp: ts("2026-09-16T09:30:00Z"), Avg: f64(20), Last: f64(20)},
		{Timestamp: ts("2026-09-16T09:45:00Z"), Avg: f64(30), Last: f64(30)},
		{Timestamp: ts("2026-09-16T10:00:00Z"), Avg: f64(40), Last: f64(40)},
	}
}

func TestRunForecastCycle_MovingAverageWritesPostgresThenRedis(t *testing.T) {
	tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{
		"cu-battery-1": avgHistory(),
	}}
	hist := &stubHistory{}
	cache := &stubCache{}
	now := ts("2026-09-16T10:07:00Z")
	h := newCycleHandler(tel, hist, cache, movingAverageTarget(true))

	res, err := h.Handle(context.Background(), RunForecastCycle{Now: now})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res.TargetsAttempted != 1 || res.TargetsSucceeded != 1 || res.TargetsFailed != 0 {
		t.Fatalf("result = %+v", res)
	}
	if len(hist.batches) != 1 || len(cache.batches) != 1 {
		t.Fatalf("saves postgres=%d redis=%d, want 1/1", len(hist.batches), len(cache.batches))
	}
	batch := hist.batches[0]
	if len(batch) != 4 {
		t.Fatalf("horizon points = %d, want 4", len(batch))
	}
	if !batch[0].GeneratedAt.Equal(now.UTC()) {
		t.Errorf("GeneratedAt = %s, want tick now", batch[0].GeneratedAt)
	}
	if !batch[0].TargetTimestamp.Equal(ts("2026-09-16T10:15:00Z")) {
		t.Errorf("first target = %s, want 10:15", batch[0].TargetTimestamp)
	}
	// mean of last 8 (only 4 available) = 25
	if batch[0].PredictedValue != 25 || batch[3].PredictedValue != 25 {
		t.Errorf("predicted = %v, want 25", batch[0].PredictedValue)
	}
	if batch[0].AlgorithmVersion != string(model.AlgorithmMovingAverage) {
		t.Errorf("AlgorithmVersion = %q", batch[0].AlgorithmVersion)
	}
	if len(tel.calls) != 1 || tel.calls[0].stepSeconds != 900 {
		t.Errorf("QueryAggregation calls = %+v", tel.calls)
	}
	if !tel.calls[0].end.Equal(now.UTC()) || !tel.calls[0].start.Equal(now.UTC().Add(-168*time.Hour)) {
		t.Errorf("window = [%s, %s]", tel.calls[0].start, tel.calls[0].end)
	}
}

func TestRunForecastCycle_PostgresFailureSkipsRedis(t *testing.T) {
	tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{
		"cu-battery-1": avgHistory(),
	}}
	hist := &stubHistory{err: errors.New("disk full")}
	cache := &stubCache{}
	h := newCycleHandler(tel, hist, cache, movingAverageTarget(true))

	res, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
	if err == nil {
		t.Fatal("expected SaveBatch error")
	}
	if res.TargetsSucceeded != 0 || res.TargetsFailed != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(cache.batches) != 0 {
		t.Fatal("Redis must not be updated when Postgres fails")
	}
}

func TestRunForecastCycle_RedisFailureStillCountsAsSuccess(t *testing.T) {
	tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{
		"cu-battery-1": avgHistory(),
	}}
	hist := &stubHistory{}
	cache := &stubCache{err: errors.New("redis down")}
	h := newCycleHandler(tel, hist, cache, movingAverageTarget(true))

	res, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
	if err != nil {
		t.Fatalf("redis failure must not fail the cycle: %v", err)
	}
	if res.TargetsSucceeded != 1 || res.RedisWriteFailed != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(hist.batches) != 1 {
		t.Fatal("Postgres must still be written")
	}
}

func TestRunForecastCycle_OneTargetFailureDoesNotBlockTheNext(t *testing.T) {
	ok := movingAverageTarget(true)
	broken := movingAverageTarget(true)
	broken.CUCode = "cu-broken"
	tel := &stubTelemetry{
		points: map[string][]port.AggregatedPoint{
			"cu-battery-1": avgHistory(),
		},
		errs: map[string]error{
			"cu-broken": errors.New("telemetry unavailable"),
		},
	}
	hist := &stubHistory{}
	cache := &stubCache{}
	h := newCycleHandler(tel, hist, cache, broken, ok)

	res, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
	if err == nil {
		t.Fatal("expected joined telemetry error")
	}
	if res.TargetsAttempted != 2 || res.TargetsSucceeded != 1 || res.TargetsFailed != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(hist.batches) != 1 || hist.batches[0][0].CUCode != "cu-battery-1" {
		t.Fatalf("expected only the healthy target to persist, got %+v", hist.batches)
	}
}

func TestRunForecastCycle_DisabledTargetsAreSkipped(t *testing.T) {
	tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{}}
	h := newCycleHandler(tel, nil, nil, movingAverageTarget(false))
	res, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res.TargetsAttempted != 0 {
		t.Fatalf("disabled target must not run, got %+v", res)
	}
	if len(tel.calls) != 0 {
		t.Fatal("disabled target must not query telemetry")
	}
}

func TestRunForecastCycle_UnknownAlgorithmFailsThatTarget(t *testing.T) {
	target := movingAverageTarget(true)
	target.Algorithm = "not_a_real_algorithm"
	h := newCycleHandler(&stubTelemetry{}, nil, nil, target)
	res, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
	if err == nil {
		t.Fatal("expected unknown-algorithm error")
	}
	if res.TargetsFailed != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestRunForecastCycle_SamePeriodPriorUsesLast(t *testing.T) {
	now := ts("2026-09-16T10:07:00Z")
	// Horizon starts at 10:15; predictor looks up 10:15 − k×24h Last values.
	tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{
		"cu-pv-1": {
			{Timestamp: ts("2026-09-14T10:15:00Z"), Avg: f64(999), Last: f64(10)},
			{Timestamp: ts("2026-09-15T10:15:00Z"), Avg: f64(999), Last: f64(20)},
			{Timestamp: ts("2026-09-14T10:30:00Z"), Avg: f64(999), Last: f64(30)},
			{Timestamp: ts("2026-09-15T10:30:00Z"), Avg: f64(999), Last: f64(50)},
			{Timestamp: ts("2026-09-14T10:45:00Z"), Avg: f64(999), Last: f64(10)},
			{Timestamp: ts("2026-09-15T10:45:00Z"), Avg: f64(999), Last: f64(10)},
			{Timestamp: ts("2026-09-14T11:00:00Z"), Avg: f64(999), Last: f64(10)},
			{Timestamp: ts("2026-09-15T11:00:00Z"), Avg: f64(999), Last: f64(10)},
		},
	}}
	hist := &stubHistory{}
	h := newCycleHandler(tel, hist, nil, samePeriodTarget("cu-pv-1"))

	res, err := h.Handle(context.Background(), RunForecastCycle{Now: now})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res.TargetsSucceeded != 1 {
		t.Fatalf("result = %+v", res)
	}
	batch := hist.batches[0]
	if batch[0].PredictedValue != 15 { // (10+20)/2 at 10:15
		t.Errorf("10:15 = %v, want 15 (Last, not Avg)", batch[0].PredictedValue)
	}
	if batch[1].PredictedValue != 40 { // (30+50)/2 at 10:30
		t.Errorf("10:30 = %v, want 40", batch[1].PredictedValue)
	}
	if batch[0].AlgorithmVersion != string(model.AlgorithmSamePeriodPrior) {
		t.Errorf("AlgorithmVersion = %q", batch[0].AlgorithmVersion)
	}
}

func TestNewRunForecastCycleHandler_PanicsWithoutTelemetry(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewRunForecastCycleHandler(nil, &stubHistory{}, &stubCache{}, nil, nil, 4, 900, time.Hour, nil, nil)
}

func TestRunForecastCycle_ObserverSeesSuccessPath(t *testing.T) {
	tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{
		"cu-battery-1": avgHistory(),
	}}
	obs := &recordingObserver{}
	h := newRunForecastCycleHandler(tel, &stubHistory{}, &stubCache{}, nil, []model.ForecastTarget{movingAverageTarget(true)}, 4, 900, 168*time.Hour, nil, obs)

	_, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if obs.cycles != 1 || obs.cycleErr {
		t.Fatalf("cycles=%d cycleErr=%v", obs.cycles, obs.cycleErr)
	}
	if obs.targetOK != 1 || obs.targetFail != 0 {
		t.Fatalf("target ok=%d fail=%d", obs.targetOK, obs.targetFail)
	}
	if obs.telemetryOK != 1 || obs.telemetryFail != 0 {
		t.Fatalf("telemetry ok=%d fail=%d", obs.telemetryOK, obs.telemetryFail)
	}
	if obs.predictOK != 1 || obs.predictFail != 0 || obs.lastPredictVersion != string(model.AlgorithmMovingAverage) {
		t.Fatalf("predict ok=%d fail=%d ver=%q", obs.predictOK, obs.predictFail, obs.lastPredictVersion)
	}
	if obs.postgresOK != 1 || obs.postgresFail != 0 {
		t.Fatalf("postgres ok=%d fail=%d", obs.postgresOK, obs.postgresFail)
	}
	if obs.redisOK != 1 || obs.redisFail != 0 {
		t.Fatalf("redis ok=%d fail=%d", obs.redisOK, obs.redisFail)
	}
}

func TestRunForecastCycle_ObserverRecordsPartialFailures(t *testing.T) {
	t.Run("telemetry", func(t *testing.T) {
		obs := &recordingObserver{}
		h := newRunForecastCycleHandler(&stubTelemetry{errs: map[string]error{"cu-battery-1": errors.New("down")}}, &stubHistory{}, &stubCache{}, nil, []model.ForecastTarget{movingAverageTarget(true)}, 4, 900, 168*time.Hour, nil, obs)
		_, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
		if err == nil {
			t.Fatal("expected telemetry error")
		}
		if obs.telemetryFail != 1 || obs.predictOK+obs.predictFail != 0 || obs.postgresOK+obs.postgresFail != 0 {
			t.Fatalf("telemetry fail=%d predict=%d/%d postgres=%d/%d", obs.telemetryFail, obs.predictOK, obs.predictFail, obs.postgresOK, obs.postgresFail)
		}
		if obs.targetFail != 1 || !obs.cycleErr {
			t.Fatalf("targetFail=%d cycleErr=%v", obs.targetFail, obs.cycleErr)
		}
	})
	t.Run("postgres skips redis", func(t *testing.T) {
		obs := &recordingObserver{}
		tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{"cu-battery-1": avgHistory()}}
		h := newRunForecastCycleHandler(tel, &stubHistory{err: errors.New("disk full")}, &stubCache{}, nil, []model.ForecastTarget{movingAverageTarget(true)}, 4, 900, 168*time.Hour, nil, obs)
		_, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
		if err == nil {
			t.Fatal("expected SaveBatch error")
		}
		if obs.postgresFail != 1 || obs.redisOK+obs.redisFail != 0 {
			t.Fatalf("postgres fail=%d redis=%d/%d", obs.postgresFail, obs.redisOK, obs.redisFail)
		}
		if obs.targetFail != 1 || !obs.cycleErr {
			t.Fatalf("targetFail=%d cycleErr=%v", obs.targetFail, obs.cycleErr)
		}
	})
	t.Run("redis failure is still a successful target", func(t *testing.T) {
		obs := &recordingObserver{}
		tel := &stubTelemetry{points: map[string][]port.AggregatedPoint{"cu-battery-1": avgHistory()}}
		h := newRunForecastCycleHandler(tel, &stubHistory{}, &stubCache{err: errors.New("redis down")}, nil, []model.ForecastTarget{movingAverageTarget(true)}, 4, 900, 168*time.Hour, nil, obs)
		_, err := h.Handle(context.Background(), RunForecastCycle{Now: ts("2026-09-16T10:07:00Z")})
		if err != nil {
			t.Fatalf("redis failure must not fail the cycle: %v", err)
		}
		if obs.postgresOK != 1 || obs.redisFail != 1 || obs.targetOK != 1 || obs.cycleErr {
			t.Fatalf("postgresOK=%d redisFail=%d targetOK=%d cycleErr=%v", obs.postgresOK, obs.redisFail, obs.targetOK, obs.cycleErr)
		}
	})
}

type recordingObserver struct {
	cycles             int
	cycleErr           bool
	targetOK           int
	targetFail         int
	predictOK          int
	predictFail        int
	lastPredictVersion string
	telemetryOK        int
	telemetryFail      int
	postgresOK         int
	postgresFail       int
	redisOK            int
	redisFail          int
}

func (r *recordingObserver) ObserveCycle(_ time.Duration, err error) {
	r.cycles++
	r.cycleErr = err != nil
}
func (r *recordingObserver) ObserveTarget(success bool) {
	if success {
		r.targetOK++
	} else {
		r.targetFail++
	}
}
func (r *recordingObserver) ObservePredict(algorithmVersion string, _ time.Duration, err error) {
	r.lastPredictVersion = algorithmVersion
	if err != nil {
		r.predictFail++
	} else {
		r.predictOK++
	}
}
func (r *recordingObserver) ObserveTelemetryQuery(success bool) {
	if success {
		r.telemetryOK++
	} else {
		r.telemetryFail++
	}
}
func (r *recordingObserver) ObservePostgresWrite(success bool) {
	if success {
		r.postgresOK++
	} else {
		r.postgresFail++
	}
}
func (r *recordingObserver) ObserveRedisWrite(success bool) {
	if success {
		r.redisOK++
	} else {
		r.redisFail++
	}
}

var _ port.Observer = (*recordingObserver)(nil)
