package query

import (
	"context"
	"errors"
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
		TenantID: "t", CUCode: "c", MetricName: "p", StartTime: start, EndTime: end,
	})
	if err != nil || !repo.called || repo.cond.MetricName != "p" {
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
		TenantID: "t", CUCode: "c", MetricName: "p",
		StartTime: start, EndTime: start.Add(40 * 24 * time.Hour),
		Step: time.Minute, Functions: []model.AggFunction{model.AggAvg},
	})
	if !errors.Is(err, types.ErrQueryRangeExceeded) {
		t.Fatalf("err = %v", err)
	}

	repo := &stubAggRepo{}
	h2 := queryAggregationHandler{aggRepo: repo}
	_, err = h2.Handle(ctx, QueryAggregation{
		TenantID: "t", CUCode: "c", MetricName: "p",
		StartTime: start, EndTime: start.Add(time.Hour),
		Step: 0, Functions: []model.AggFunction{model.AggAvg},
	})
	if err == nil || repo.called {
		t.Fatalf("want step validation before repo, err=%v called=%v", err, repo.called)
	}

	_, err = h2.Handle(ctx, QueryAggregation{
		TenantID: "t", CUCode: "c", MetricName: "p",
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
	v := snapshotToView(s, 5*time.Minute)
	if !v.Stale || v.CUCode != "cu" {
		t.Fatalf("view = %+v", v)
	}
	v2 := snapshotToView(s, 0)
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
