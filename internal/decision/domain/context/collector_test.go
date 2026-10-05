package context

import (
	stdctx "context"
	"errors"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestSnapshotCollector_SkipsStaleMissingAndBadQuality(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	scope := port.ResolvedScope{Members: []port.ResolvedCU{{CUID: "cu-1"}}}
	tel := &fakeTelemetry{}
	collector := NewSnapshotCollector(tel)

	tel.snaps = []port.CUSnapshot{sample("cu-1", 15, at, port.QualityGood, true)}
	_, err := collector.Collect(stdctx.Background(), "tenant-1", scope, socMetric(), time.Minute, at)
	if !errors.Is(err, ErrUnusableState) {
		t.Fatalf("stale err = %v", err)
	}

	tel.snaps = []port.CUSnapshot{sample("cu-1", 15, at, port.QualityBad, false)}
	_, err = collector.Collect(stdctx.Background(), "tenant-1", scope, socMetric(), time.Minute, at)
	if !errors.Is(err, ErrUnusableState) {
		t.Fatalf("quality err = %v", err)
	}

	tel.snaps = []port.CUSnapshot{{CUCode: "cu-1"}}
	_, err = collector.Collect(stdctx.Background(), "tenant-1", scope, socMetric(), time.Minute, at)
	if !errors.Is(err, ErrUnusableState) {
		t.Fatalf("missing err = %v", err)
	}

	tel.snaps = []port.CUSnapshot{sample("cu-1", 15, at.Add(-2*time.Minute), port.QualityGood, false)}
	_, err = collector.Collect(stdctx.Background(), "tenant-1", scope, socMetric(), time.Minute, at)
	if !errors.Is(err, ErrUnusableState) {
		t.Fatalf("aged err = %v", err)
	}
}

func TestSnapshotCollector_BatchesByTenantAndSkipsOnlyTheBadScope(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tel := &fakeTelemetry{snaps: []port.CUSnapshot{
		sample("cu-ok", 40, at, port.QualityGood, false),
		sample("cu-old", 5, at, port.QualityGood, true),
	}}
	collector := NewSnapshotCollector(tel)
	scopes := []port.ResolvedScope{
		{Members: []port.ResolvedCU{{CUID: "cu-ok"}}},
		{Members: []port.ResolvedCU{{CUID: "cu-old"}}},
	}
	got, err := collector.CollectBatch(stdctx.Background(), "tenant-1", scopes, socMetric(), time.Minute, at)
	if err != nil {
		t.Fatal(err)
	}
	if tel.calls != 1 {
		t.Fatalf("snapshot calls = %d, want 1", tel.calls)
	}
	if got[0].Err != nil || got[0].State.Units[0].Metrics[0].Value != 40 {
		t.Fatalf("good scope = %+v", got[0])
	}
	if !errors.Is(got[1].Err, ErrUnusableState) {
		t.Fatalf("stale scope err = %v", got[1].Err)
	}
}

func TestSnapshotCollector_TransportErrorFailsTheBatch(t *testing.T) {
	tel := &fakeTelemetry{err: errors.New("unavailable")}
	collector := NewSnapshotCollector(tel)
	_, err := collector.Collect(
		stdctx.Background(),
		"tenant-1",
		port.ResolvedScope{Members: []port.ResolvedCU{{CUID: "cu-1"}}},
		socMetric(),
		time.Minute,
		time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	)
	if err == nil || errors.Is(err, ErrUnusableState) {
		t.Fatalf("err = %v", err)
	}
}

type fakeTelemetry struct {
	snaps []port.CUSnapshot
	err   error
	calls int
}

func (f *fakeTelemetry) GetSnapshots(stdctx.Context, port.SnapshotQuery) ([]port.CUSnapshot, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.snaps, nil
}

func socMetric() []contracts.MetricID {
	return []contracts.MetricID{contracts.MetricEnergyStorageStateOfCharge}
}

func sample(cu string, soc float64, at time.Time, quality port.Quality, stale bool) port.CUSnapshot {
	return port.CUSnapshot{
		CUCode: cu,
		Stale:  stale,
		Metrics: []port.MetricSample{{
			MetricID:   contracts.MetricEnergyStorageStateOfCharge,
			Value:      soc,
			ObservedAt: at,
			Quality:    quality,
		}},
	}
}
