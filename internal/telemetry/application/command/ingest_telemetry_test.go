package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

type stubTelemetryRepo struct {
	saved []*model.TelemetryRecord
	err   error
}

func (r *stubTelemetryRepo) SaveBatch(_ context.Context, records []*model.TelemetryRecord) error {
	if r.err != nil {
		return r.err
	}
	r.saved = append(r.saved, records...)
	return nil
}

func (r *stubTelemetryRepo) Query(context.Context, model.QueryCondition) ([]*model.TelemetryRecord, error) {
	return nil, errors.New("not implemented")
}

type stubSnapshotRepo struct {
	snap    *model.Snapshot
	findErr error
	saveErr error
	saved   *model.Snapshot
	saveN   int
}

func (r *stubSnapshotRepo) Save(_ context.Context, snapshot *model.Snapshot) error {
	r.saveN++
	if r.saveErr != nil {
		return r.saveErr
	}
	cp := *snapshot
	metrics := make(map[string]float64, len(snapshot.Metrics))
	for k, v := range snapshot.Metrics {
		metrics[k] = v
	}
	cp.Metrics = metrics
	r.saved = &cp
	return nil
}

func (r *stubSnapshotRepo) Find(context.Context, string, string) (*model.Snapshot, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.snap, nil
}

func (r *stubSnapshotRepo) FindAll(context.Context, string) ([]*model.Snapshot, error) {
	return nil, errors.New("not implemented")
}

type stubPublisher struct {
	events []*model.SOEEvent
	err    error
}

func (p *stubPublisher) PublishSOE(_ context.Context, event *model.SOEEvent) error {
	p.events = append(p.events, event)
	return p.err
}

var (
	_ port.TelemetryRepository = (*stubTelemetryRepo)(nil)
	_ port.SnapshotRepository  = (*stubSnapshotRepo)(nil)
	_ port.EventPublisher      = (*stubPublisher)(nil)
)

func TestIngestTelemetry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := time.Unix(1700000000, 0).UTC()

	t.Run("happy path with discrete SOE", func(t *testing.T) {
		t.Parallel()
		base := model.NewSnapshot("tenant", "cu-1")
		base.Metrics["brk"] = 0
		base.UpdatedAt = ts.Add(-time.Minute)

		tel := &stubTelemetryRepo{}
		snap := &stubSnapshotRepo{snap: base}
		pub := &stubPublisher{}
		h := ingestTelemetryHandler{telemetryRepo: tel, snapshotRepo: snap, publisher: pub}

		res, err := h.Handle(ctx, IngestTelemetry{
			TenantID: "tenant", CUCode: "cu-1", Timestamp: ts,
			Metrics: []MetricInput{
				{Name: "power", Value: 12, Type: model.Analog, Quality: model.QualityGood},
				{Name: "brk", Value: 1, Type: model.Discrete, Quality: model.QualityGood},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.SOECount != 1 || len(pub.events) != 1 {
			t.Fatalf("SOECount=%d pub=%d", res.SOECount, len(pub.events))
		}
		if len(tel.saved) != 1 || snap.saveN != 1 {
			t.Fatalf("saved tel=%d snap=%d", len(tel.saved), snap.saveN)
		}
		if snap.saved.Metrics["brk"] != 1 || snap.saved.Metrics["power"] != 12 {
			t.Fatalf("snapshot = %+v", snap.saved.Metrics)
		}
	})

	t.Run("invalid record", func(t *testing.T) {
		t.Parallel()
		tel := &stubTelemetryRepo{}
		h := ingestTelemetryHandler{
			telemetryRepo: tel,
			snapshotRepo:  &stubSnapshotRepo{findErr: domain.ErrSnapshotNotFound},
			publisher:     &stubPublisher{},
		}
		_, err := h.Handle(ctx, IngestTelemetry{TenantID: "t", CUCode: "c", Timestamp: ts})
		if err == nil || len(tel.saved) != 0 {
			t.Fatalf("err=%v saved=%d", err, len(tel.saved))
		}
	})

	t.Run("SaveBatch failure is hard gate", func(t *testing.T) {
		t.Parallel()
		tel := &stubTelemetryRepo{err: errors.New("tsdb down")}
		snap := &stubSnapshotRepo{findErr: domain.ErrSnapshotNotFound}
		pub := &stubPublisher{}
		h := ingestTelemetryHandler{telemetryRepo: tel, snapshotRepo: snap, publisher: pub}
		_, err := h.Handle(ctx, IngestTelemetry{
			TenantID: "t", CUCode: "c", Timestamp: ts,
			Metrics: []MetricInput{{Name: "p", Value: 1, Type: model.Analog, Quality: model.QualityGood}},
		})
		if err == nil || snap.saveN != 0 || len(pub.events) != 0 {
			t.Fatalf("err=%v snapSave=%d pub=%d", err, snap.saveN, len(pub.events))
		}
	})

	t.Run("snapshot not found uses empty baseline", func(t *testing.T) {
		t.Parallel()
		tel := &stubTelemetryRepo{}
		snap := &stubSnapshotRepo{findErr: domain.ErrSnapshotNotFound}
		pub := &stubPublisher{}
		h := ingestTelemetryHandler{telemetryRepo: tel, snapshotRepo: snap, publisher: pub}
		res, err := h.Handle(ctx, IngestTelemetry{
			TenantID: "t", CUCode: "c", Timestamp: ts,
			Metrics: []MetricInput{{Name: "brk", Value: 1, Type: model.Discrete, Quality: model.QualityGood}},
		})
		if err != nil {
			t.Fatal(err)
		}
		// First discrete write on empty baseline → no SOE.
		if res.SOECount != 0 || len(pub.events) != 0 {
			t.Fatalf("SOECount=%d", res.SOECount)
		}
		if snap.saved == nil || snap.saved.Metrics["brk"] != 1 {
			t.Fatalf("saved = %+v", snap.saved)
		}
	})

	t.Run("snapshot read error still succeeds", func(t *testing.T) {
		t.Parallel()
		tel := &stubTelemetryRepo{}
		snap := &stubSnapshotRepo{findErr: errors.New("redis down")}
		pub := &stubPublisher{}
		h := ingestTelemetryHandler{telemetryRepo: tel, snapshotRepo: snap, publisher: pub}
		res, err := h.Handle(ctx, IngestTelemetry{
			TenantID: "t", CUCode: "c", Timestamp: ts,
			Metrics: []MetricInput{{Name: "p", Value: 1, Type: model.Analog, Quality: model.QualityGood}},
		})
		if err != nil || res == nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("snapshot write failure still succeeds and publishes SOE", func(t *testing.T) {
		t.Parallel()
		base := model.NewSnapshot("t", "c")
		base.Metrics["brk"] = 0
		tel := &stubTelemetryRepo{}
		snap := &stubSnapshotRepo{snap: base, saveErr: errors.New("redis write")}
		pub := &stubPublisher{}
		h := ingestTelemetryHandler{telemetryRepo: tel, snapshotRepo: snap, publisher: pub}
		res, err := h.Handle(ctx, IngestTelemetry{
			TenantID: "t", CUCode: "c", Timestamp: ts,
			Metrics: []MetricInput{{Name: "brk", Value: 1, Type: model.Discrete, Quality: model.QualityGood}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.SOECount != 1 || len(pub.events) != 1 {
			t.Fatalf("want SOE despite snapshot write fail: count=%d pub=%d", res.SOECount, len(pub.events))
		}
	})

	t.Run("publish failure ignored", func(t *testing.T) {
		t.Parallel()
		base := model.NewSnapshot("t", "c")
		base.Metrics["brk"] = 0
		h := ingestTelemetryHandler{
			telemetryRepo: &stubTelemetryRepo{},
			snapshotRepo:  &stubSnapshotRepo{snap: base},
			publisher:     &stubPublisher{err: errors.New("kafka down")},
		}
		res, err := h.Handle(ctx, IngestTelemetry{
			TenantID: "t", CUCode: "c", Timestamp: ts,
			Metrics: []MetricInput{{Name: "brk", Value: 1, Type: model.Discrete, Quality: model.QualityGood}},
		})
		if err != nil || res.SOECount != 1 {
			t.Fatalf("err=%v SOECount=%d", err, res.SOECount)
		}
	})
}
