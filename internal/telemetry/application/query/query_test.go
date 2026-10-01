package query

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/application/types"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

type stubTelemetryRepo struct {
	called bool
	cond   model.QueryCondition
	out    []*model.TelemetryRecord
	err    error
}

func (r *stubTelemetryRepo) SaveBatch(context.Context, []*model.TelemetryRecord) error {
	return errors.New("not implemented")
}

func (r *stubTelemetryRepo) Query(_ context.Context, condition model.QueryCondition) ([]*model.TelemetryRecord, error) {
	r.called = true
	r.cond = condition
	return r.out, r.err
}

type stubAggRepo struct {
	called bool
	q      model.AggregationQuery
	out    []*model.AggregatedPoint
	err    error
}

func (r *stubAggRepo) Query(_ context.Context, q model.AggregationQuery) ([]*model.AggregatedPoint, error) {
	r.called = true
	r.q = q
	return r.out, r.err
}

type stubSnapshotRepo struct {
	snap *model.Snapshot
	err  error
}

func (r *stubSnapshotRepo) Save(context.Context, *model.Snapshot) error { return nil }
func (r *stubSnapshotRepo) Find(context.Context, string, string) (*model.Snapshot, error) {
	return r.snap, r.err
}
func (r *stubSnapshotRepo) FindAll(context.Context, string) ([]*model.Snapshot, error) {
	return nil, errors.New("not implemented")
}

func (r *stubSnapshotRepo) FindByCUs(_ context.Context, tenantID string, cuCodes []string) ([]*model.Snapshot, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.snap == nil {
		return nil, nil
	}
	out := make([]*model.Snapshot, 0, len(cuCodes))
	for _, cu := range cuCodes {
		if r.snap.TenantID == tenantID && r.snap.CUCode == cu {
			out = append(out, r.snap)
		}
	}
	return out, nil
}

var (
	_ port.TelemetryRepository   = (*stubTelemetryRepo)(nil)
	_ port.AggregationRepository = (*stubAggRepo)(nil)
	_ port.SnapshotRepository    = (*stubSnapshotRepo)(nil)
)

func TestQueryTelemetry_RangePolicy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := queryTelemetryHandler{telemetryRepo: &stubTelemetryRepo{}}

	start := time.Unix(0, 0)
	_, err := h.Handle(ctx, QueryTelemetry{
		TenantID: "t", CUCode: "c", StartTime: start, EndTime: start.Add(31 * 24 * time.Hour),
	})
	if !errors.Is(err, types.ErrQueryRangeExceeded) {
		t.Fatalf("err = %v", err)
	}

	repo := &stubTelemetryRepo{out: []*model.TelemetryRecord{}}
	h2 := queryTelemetryHandler{telemetryRepo: repo}
	end := start.Add(24 * time.Hour)
	_, err = h2.Handle(ctx, QueryTelemetry{
		TenantID: "t", CUCode: "c", MetricID: "electrical.active_power.v1", StartTime: start, EndTime: end,
	})
	if err != nil || !repo.called || repo.cond.MetricID != "electrical.active_power.v1" {
		t.Fatalf("err=%v called=%v cond=%+v", err, repo.called, repo.cond)
	}

	repo3 := &stubTelemetryRepo{}
	h3 := queryTelemetryHandler{telemetryRepo: repo3}
	_, err = h3.Handle(ctx, QueryTelemetry{
		TenantID: "", CUCode: "c", StartTime: start, EndTime: end,
	})
	if err == nil {
		t.Fatal("want domain validation error")
	}
	if repo3.called {
		t.Fatal("should not call repo on invalid condition")
	}
}

func TestQueryAggregation_RangeAndValidate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	start := time.Unix(0, 0)

	h := queryAggregationHandler{aggRepo: &stubAggRepo{}}
	_, err := h.Handle(ctx, QueryAggregation{
		TenantID: "t", CUCode: "c", MetricID: "electrical.active_power.v1",
		StartTime: start, EndTime: start.Add(40 * 24 * time.Hour),
		Step: time.Minute, Functions: []model.AggFunction{model.AggAvg},
	})
	if !errors.Is(err, types.ErrQueryRangeExceeded) {
		t.Fatalf("err = %v", err)
	}

	repo := &stubAggRepo{}
	h2 := queryAggregationHandler{aggRepo: repo}
	_, err = h2.Handle(ctx, QueryAggregation{
		TenantID: "t", CUCode: "c", MetricID: "electrical.active_power.v1",
		StartTime: start, EndTime: start.Add(time.Hour),
		Step: 0, Functions: []model.AggFunction{model.AggAvg},
	})
	if err == nil || repo.called {
		t.Fatalf("want step validation before repo, err=%v called=%v", err, repo.called)
	}

	_, err = h2.Handle(ctx, QueryAggregation{
		TenantID: "t", CUCode: "c", MetricID: "electrical.active_power.v1",
		StartTime: start, EndTime: start.Add(time.Hour),
		Step: time.Minute, Functions: []model.AggFunction{model.AggMax},
	})
	if err != nil || !repo.called || repo.q.Functions[0] != model.AggMax {
		t.Fatalf("err=%v called=%v q=%+v", err, repo.called, repo.q)
	}
}

func TestSnapshotToViewAndGetSnapshot(t *testing.T) {
	t.Parallel()

	s := model.NewSnapshot("t", "cu")
	s.UpdatedAt = time.Now().Add(-10 * time.Minute)
	v := snapshotToView(s, nil, 5*time.Minute)
	if !v.Stale || v.CUCode != "cu" {
		t.Fatalf("view = %+v", v)
	}
	v2 := snapshotToView(s, nil, 0)
	if v2.Stale {
		t.Fatal("staleAge 0 should skip check")
	}

	repo := &stubSnapshotRepo{snap: s}
	h := getSnapshotHandler{snapshotRepo: repo}
	got, err := h.Handle(context.Background(), GetSnapshot{TenantID: "t", CUCode: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Stale {
		t.Fatal("default stale age should mark stale")
	}
	got, err = h.Handle(context.Background(), GetSnapshot{
		TenantID: "t", CUCode: "cu", StaleAge: time.Hour,
	})
	if err != nil || got.Stale {
		t.Fatalf("custom age: stale=%v err=%v", got.Stale, err)
	}
}

func TestGetSnapshots_FiltersMetricsAndSkipsMissingCUs(t *testing.T) {
	t.Parallel()
	observed := time.Unix(1700000000, 0).UTC()
	snap := model.NewSnapshot("t", "cu-1")
	snap.UpdatedAt = time.Now()
	snap.Metrics = map[string]model.MetricState{
		"electrical.active_power.v1": {
			MetricID: "electrical.active_power.v1", Value: 12, ObservedAt: observed, Quality: model.QualityGood,
		},
		"energy_storage.state_of_charge.v1": {
			MetricID: "energy_storage.state_of_charge.v1", Value: 40, ObservedAt: observed, Quality: model.QualityBad,
		},
	}
	h := getSnapshotsHandler{snapshotRepo: &stubSnapshotRepo{snap: snap}}
	views, err := h.Handle(context.Background(), GetSnapshots{
		TenantID:  "t",
		CUCodes:   []string{"cu-missing", "cu-1", "cu-1"},
		MetricIDs: []string{"energy_storage.state_of_charge.v1", "electrical.active_power_setpoint.v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].CUCode != "cu-1" || len(views[0].Metrics) != 1 {
		t.Fatalf("views = %+v", views)
	}
	got := views[0].Metrics[0]
	if got.MetricID != "energy_storage.state_of_charge.v1" || got.Value != 40 || got.Quality != model.QualityBad {
		t.Fatalf("metric = %+v", got)
	}
	if !got.ObservedAt.Equal(observed) {
		t.Fatalf("observed = %v", got.ObservedAt)
	}

	_, err = h.Handle(context.Background(), GetSnapshots{TenantID: "t", CUCodes: []string{"cu-1"}})
	if err == nil || !strings.Contains(err.Error(), "metric_id") {
		t.Fatalf("err = %v", err)
	}
	_, err = h.Handle(context.Background(), GetSnapshots{
		TenantID: "t", CUCodes: []string{"cu-1"}, MetricIDs: []string{"soc"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid metric id") {
		t.Fatalf("err = %v", err)
	}
}
