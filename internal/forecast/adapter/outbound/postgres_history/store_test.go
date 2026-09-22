package postgreshistory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

func TestSQLContract(t *testing.T) {
	if !strings.Contains(insertSQL, "ON CONFLICT") || !strings.Contains(insertSQL, "DO NOTHING") {
		t.Fatal("SaveBatch must be idempotent on the unique batch key")
	}
	if !strings.Contains(latestGeneratedAtSQL, "MAX(generated_at)") {
		t.Fatal("GetLatestBatch step 1 must take MAX(generated_at)")
	}
	if !strings.Contains(latestBatchSQL, "generated_at = $4") {
		t.Fatal("GetLatestBatch step 2 must filter on the latest generated_at")
	}
	if !strings.Contains(queryLatestPerTargetSQL, "DISTINCT ON (target_timestamp)") {
		t.Fatal("Query with unset GeneratedAt must collapse to the latest generation per target_timestamp")
	}
	if !strings.Contains(queryByGeneratedAtSQL, "generated_at = $6") {
		t.Fatal("Query with GeneratedAt set must pin the batch")
	}
}

func TestNewPool_RequiresHostOrDSN(t *testing.T) {
	if _, err := NewPool(context.Background(), platformpostgres.Config{}); err == nil {
		t.Fatal("expected error when host and dsn are empty")
	}
}

func TestSaveBatch_EmptyIsNoOp(t *testing.T) {
	store := newStore(&stubPool{})
	if err := store.SaveBatch(context.Background(), nil); err != nil {
		t.Fatalf("empty batch: %v", err)
	}
}

func TestSaveBatch_RejectsMixedIdentity(t *testing.T) {
	store := newStore(&stubPool{})
	batch := sampleHistoryBatch()
	batch[1].CUCode = "other"
	if err := store.SaveBatch(context.Background(), batch); err == nil {
		t.Fatal("expected mixed-identity error")
	}
}

func TestSaveBatch_QueuesOneInsertPerPoint(t *testing.T) {
	pool := &stubPool{}
	store := newStore(pool)
	batch := sampleHistoryBatch()
	if err := store.SaveBatch(context.Background(), batch); err != nil {
		t.Fatalf("SaveBatch: %v", err)
	}
	if pool.batch == nil || pool.batch.Len() != len(batch) {
		t.Fatalf("queued %v, want %d inserts", pool.batch, len(batch))
	}
	if pool.batchResults.execs != len(batch) {
		t.Fatalf("Exec calls = %d, want %d", pool.batchResults.execs, len(batch))
	}
}

func TestGetLatestBatch_NeverForecastIsNilNil(t *testing.T) {
	store := newStore(&stubPool{row: stubRow{vals: []any{nil}}})
	got, err := store.GetLatestBatch(context.Background(), "tenant-a", "cu-battery-1", "active_power_kw")
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if got != nil {
		t.Fatalf("never-forecast must be nil, got %+v", got)
	}
}

func TestGetLatestBatch_ReturnsWholeLatestBatch(t *testing.T) {
	generated := time.Date(2026, 9, 16, 10, 7, 0, 0, time.UTC)
	t15 := time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC)
	t30 := time.Date(2026, 9, 16, 10, 30, 0, 0, time.UTC)
	pool := &stubPool{
		row: stubRow{vals: []any{generated}},
		rows: &stubRows{data: [][]any{
			{"tenant-a", "cu-battery-1", "active_power_kw", generated, t15, 100.0, "moving_average"},
			{"tenant-a", "cu-battery-1", "active_power_kw", generated, t30, 100.0, "moving_average"},
		}},
	}
	store := newStore(pool)
	got, err := store.GetLatestBatch(context.Background(), "tenant-a", "cu-battery-1", "active_power_kw")
	if err != nil {
		t.Fatalf("GetLatestBatch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[0].TargetTimestamp.Equal(t15) || !got[1].TargetTimestamp.Equal(t30) {
		t.Errorf("timestamps = %s, %s", got[0].TargetTimestamp, got[1].TargetTimestamp)
	}
	if pool.querySQL != latestBatchSQL {
		t.Errorf("step 2 SQL was not latestBatchSQL")
	}
}

func TestQuery_ValidatesWindow(t *testing.T) {
	store := newStore(&stubPool{})
	_, err := store.Query(context.Background(), port.HistoryQuery{
		TenantID: "t", CUCode: "cu", MetricName: "kw",
	})
	if err == nil {
		t.Fatal("zero window must error")
	}
	start := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	_, err = store.Query(context.Background(), port.HistoryQuery{
		TenantID: "t", CUCode: "cu", MetricName: "kw",
		StartTime: start, EndTime: start.Add(-time.Hour),
	})
	if err == nil {
		t.Fatal("inverted window must error")
	}
}

func TestQuery_UnsetGeneratedAtUsesLatestPerTarget(t *testing.T) {
	start := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	generated := time.Date(2026, 9, 16, 10, 7, 0, 0, time.UTC)
	target := time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC)
	pool := &stubPool{
		rows: &stubRows{data: [][]any{
			{"tenant-a", "cu-battery-1", "active_power_kw", generated, target, 80.0, "same_period_prior"},
		}},
	}
	store := newStore(pool)
	got, err := store.Query(context.Background(), port.HistoryQuery{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		StartTime: start, EndTime: end,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 || got[0].PredictedValue != 80 {
		t.Fatalf("got %+v", got)
	}
	if pool.querySQL != queryLatestPerTargetSQL {
		t.Error("expected DISTINCT ON latest-per-target SQL")
	}
}

func TestQuery_GeneratedAtPinsTheBatch(t *testing.T) {
	start := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	generated := time.Date(2026, 9, 16, 6, 7, 0, 0, time.UTC)
	pool := &stubPool{rows: &stubRows{}}
	store := newStore(pool)
	got, err := store.Query(context.Background(), port.HistoryQuery{
		TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
		StartTime: start, EndTime: end, GeneratedAt: generated,
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got == nil {
		t.Fatal("empty result must be a non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
	if pool.querySQL != queryByGeneratedAtSQL {
		t.Error("expected generated_at-pinned SQL")
	}
	if len(pool.queryArgs) != 6 {
		t.Fatalf("args = %d, want 6", len(pool.queryArgs))
	}
}

func TestGetLatestBatch_RequiresIdentity(t *testing.T) {
	store := newStore(&stubPool{})
	if _, err := store.GetLatestBatch(context.Background(), "", "cu", "kw"); err == nil {
		t.Fatal("empty tenant must error")
	}
}

func TestSaveBatch_PropagatesExecError(t *testing.T) {
	store := newStore(&stubPool{execErr: errors.New("disk full")})
	if err := store.SaveBatch(context.Background(), sampleHistoryBatch()); err == nil {
		t.Fatal("expected exec error")
	}
}

func sampleHistoryBatch() []model.Prediction {
	generated := time.Date(2026, 9, 16, 10, 7, 0, 0, time.UTC)
	return []model.Prediction{
		{
			TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
			GeneratedAt: generated, TargetTimestamp: time.Date(2026, 9, 16, 10, 15, 0, 0, time.UTC),
			PredictedValue: 100, AlgorithmVersion: "moving_average",
		},
		{
			TenantID: "tenant-a", CUCode: "cu-battery-1", MetricName: "active_power_kw",
			GeneratedAt: generated, TargetTimestamp: time.Date(2026, 9, 16, 10, 30, 0, 0, time.UTC),
			PredictedValue: 100, AlgorithmVersion: "moving_average",
		},
	}
}

type stubPool struct {
	batch        *pgx.Batch
	batchResults *stubBatchResults
	row          pgx.Row
	rows         pgx.Rows
	querySQL     string
	queryArgs    []any
	execErr      error
}

func (p *stubPool) SendBatch(_ context.Context, b *pgx.Batch) pgx.BatchResults {
	p.batch = b
	p.batchResults = &stubBatchResults{err: p.execErr}
	return p.batchResults
}

func (p *stubPool) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	p.querySQL = sql
	p.queryArgs = args
	if p.rows == nil {
		return &stubRows{}, nil
	}
	return p.rows, nil
}

func (p *stubPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if p.row == nil {
		return stubRow{err: errors.New("no row configured")}
	}
	return p.row
}

type stubBatchResults struct {
	execs int
	err   error
}

func (b *stubBatchResults) Exec() (pgconn.CommandTag, error) {
	b.execs++
	return pgconn.NewCommandTag("INSERT 0 1"), b.err
}
func (b *stubBatchResults) Query() (pgx.Rows, error) { panic("unused") }
func (b *stubBatchResults) QueryRow() pgx.Row        { panic("unused") }
func (b *stubBatchResults) Close() error             { return nil }

type stubRow struct {
	vals []any
	err  error
}

func (r stubRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return assignDest(dest, r.vals)
}

type stubRows struct {
	data [][]any
	idx  int
	err  error
}

func (r *stubRows) Close()                                       {}
func (r *stubRows) Err() error                                   { return r.err }
func (r *stubRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *stubRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *stubRows) Values() ([]any, error)                       { return nil, nil }
func (r *stubRows) RawValues() [][]byte                          { return nil }
func (r *stubRows) Conn() *pgx.Conn                              { return nil }

func (r *stubRows) Next() bool {
	if r.idx >= len(r.data) {
		return false
	}
	r.idx++
	return true
}

func (r *stubRows) Scan(dest ...any) error {
	if r.idx == 0 || r.idx > len(r.data) {
		return errors.New("scan called without Next")
	}
	return assignDest(dest, r.data[r.idx-1])
}

func assignDest(dest []any, vals []any) error {
	if len(dest) != len(vals) {
		return errors.New("dest/vals length mismatch")
	}
	for i, d := range dest {
		switch ptr := d.(type) {
		case *string:
			s, ok := vals[i].(string)
			if !ok {
				return errors.New("expected string")
			}
			*ptr = s
		case *float64:
			f, ok := vals[i].(float64)
			if !ok {
				return errors.New("expected float64")
			}
			*ptr = f
		case *time.Time:
			tm, ok := vals[i].(time.Time)
			if !ok {
				return errors.New("expected time.Time")
			}
			*ptr = tm
		case **time.Time:
			if vals[i] == nil {
				*ptr = nil
				continue
			}
			tm, ok := vals[i].(time.Time)
			if !ok {
				return errors.New("expected time.Time")
			}
			cp := tm
			*ptr = &cp
		default:
			return errors.New("unsupported dest type")
		}
	}
	return nil
}
