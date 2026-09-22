package postgreshistory

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

// Store implements port.HistoryPort against forecast_history.
type Store struct {
	pool pool
}

// pool is *pgxpool.Pool in production. Tests inject a fake.
type pool interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// NewStore wraps pool. Prefer NewPool at the composition root so the
// process owns a single connection pool.
func NewStore(p *pgxpool.Pool) *Store {
	return &Store{pool: p}
}

func newStore(p pool) *Store {
	return &Store{pool: p}
}

var _ port.HistoryPort = (*Store)(nil)

const insertSQL = `
INSERT INTO forecast_history (
    id, tenant_id, cu_code, metric_name,
    generated_at, target_timestamp, predicted_value, algorithm_version
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (tenant_id, cu_code, metric_name, generated_at, target_timestamp)
DO NOTHING`

const latestGeneratedAtSQL = `
SELECT MAX(generated_at) AS latest_generated_at
FROM forecast_history
WHERE tenant_id = $1 AND cu_code = $2 AND metric_name = $3`

const latestBatchSQL = `
SELECT tenant_id, cu_code, metric_name, generated_at, target_timestamp, predicted_value, algorithm_version
FROM forecast_history
WHERE tenant_id = $1 AND cu_code = $2 AND metric_name = $3
  AND generated_at = $4
ORDER BY target_timestamp ASC`

const queryByGeneratedAtSQL = `
SELECT tenant_id, cu_code, metric_name, generated_at, target_timestamp, predicted_value, algorithm_version
FROM forecast_history
WHERE tenant_id = $1 AND cu_code = $2 AND metric_name = $3
  AND target_timestamp >= $4 AND target_timestamp <= $5
  AND generated_at = $6
ORDER BY target_timestamp ASC`

// queryLatestPerTargetSQL: GeneratedAt unset — one row per
// target_timestamp, the most recently generated (design plan §3 / proto).
const queryLatestPerTargetSQL = `
SELECT DISTINCT ON (target_timestamp)
    tenant_id, cu_code, metric_name, generated_at, target_timestamp, predicted_value, algorithm_version
FROM forecast_history
WHERE tenant_id = $1 AND cu_code = $2 AND metric_name = $3
  AND target_timestamp >= $4 AND target_timestamp <= $5
ORDER BY target_timestamp ASC, generated_at DESC`

// SaveBatch writes one forecast cycle's horizon. Empty batch is a no-op.
// ON CONFLICT DO NOTHING makes a retry of the same (generated_at,
// target_timestamp) set idempotent.
func (s *Store) SaveBatch(ctx context.Context, batch []model.Prediction) error {
	if len(batch) == 0 {
		return nil
	}
	if err := requireBatchIdentity(batch); err != nil {
		return fmt.Errorf("postgres_history: SaveBatch: %w", err)
	}

	pgxBatch := &pgx.Batch{}
	for _, p := range batch {
		id, err := idgen.NewUUIDv7()
		if err != nil {
			return fmt.Errorf("postgres_history: SaveBatch: id: %w", err)
		}
		pgxBatch.Queue(insertSQL,
			id,
			p.TenantID, p.CUCode, p.MetricName,
			p.GeneratedAt.UTC(), p.TargetTimestamp.UTC(),
			p.PredictedValue, p.AlgorithmVersion,
		)
	}

	results := s.pool.SendBatch(ctx, pgxBatch)
	defer func() { _ = results.Close() }()

	for range batch {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("postgres_history: SaveBatch: %w", err)
		}
	}
	return nil
}

// GetLatestBatch returns every point of the most recent generated_at
// for this target. It does not filter on target_timestamp — that is
// SelectNextPoint's job (design plan §7.2). Never-forecast is (nil, nil).
func (s *Store) GetLatestBatch(ctx context.Context, tenantID, cuCode, metricName string) ([]model.Prediction, error) {
	if err := requireIdentity(tenantID, cuCode, metricName); err != nil {
		return nil, fmt.Errorf("postgres_history: GetLatestBatch: %w", err)
	}

	var generatedAt *time.Time
	if err := s.pool.QueryRow(ctx, latestGeneratedAtSQL, tenantID, cuCode, metricName).Scan(&generatedAt); err != nil {
		return nil, fmt.Errorf("postgres_history: GetLatestBatch: latest generated_at: %w", err)
	}
	if generatedAt == nil {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, latestBatchSQL, tenantID, cuCode, metricName, generatedAt.UTC())
	if err != nil {
		return nil, fmt.Errorf("postgres_history: GetLatestBatch: %w", err)
	}
	defer rows.Close()

	out, err := scanPredictions(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres_history: GetLatestBatch: %w", err)
	}
	return out, nil
}

// Query returns forecast_history rows in the target_timestamp window.
func (s *Store) Query(ctx context.Context, q port.HistoryQuery) ([]model.Prediction, error) {
	if err := validateHistoryQuery(q); err != nil {
		return nil, fmt.Errorf("postgres_history: Query: %w", err)
	}

	var (
		rows pgx.Rows
		err  error
	)
	if q.GeneratedAt.IsZero() {
		rows, err = s.pool.Query(ctx, queryLatestPerTargetSQL,
			q.TenantID, q.CUCode, q.MetricName,
			q.StartTime.UTC(), q.EndTime.UTC(),
		)
	} else {
		rows, err = s.pool.Query(ctx, queryByGeneratedAtSQL,
			q.TenantID, q.CUCode, q.MetricName,
			q.StartTime.UTC(), q.EndTime.UTC(), q.GeneratedAt.UTC(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres_history: Query: %w", err)
	}
	defer rows.Close()

	out, err := scanPredictions(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres_history: Query: %w", err)
	}
	if out == nil {
		out = []model.Prediction{}
	}
	return out, nil
}

func scanPredictions(rows pgx.Rows) ([]model.Prediction, error) {
	var out []model.Prediction
	for rows.Next() {
		var p model.Prediction
		if err := rows.Scan(
			&p.TenantID, &p.CUCode, &p.MetricName,
			&p.GeneratedAt, &p.TargetTimestamp,
			&p.PredictedValue, &p.AlgorithmVersion,
		); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		p.GeneratedAt = p.GeneratedAt.UTC()
		p.TargetTimestamp = p.TargetTimestamp.UTC()
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func validateHistoryQuery(q port.HistoryQuery) error {
	if err := requireIdentity(q.TenantID, q.CUCode, q.MetricName); err != nil {
		return err
	}
	if q.StartTime.IsZero() || q.EndTime.IsZero() {
		return fmt.Errorf("start_time and end_time are required")
	}
	if q.StartTime.After(q.EndTime) {
		return fmt.Errorf("start_time cannot be after end_time")
	}
	return nil
}

func requireBatchIdentity(batch []model.Prediction) error {
	first := batch[0]
	if err := requireIdentity(first.TenantID, first.CUCode, first.MetricName); err != nil {
		return err
	}
	if first.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is required")
	}
	for i, p := range batch {
		if p.TenantID != first.TenantID || p.CUCode != first.CUCode || p.MetricName != first.MetricName {
			return fmt.Errorf("point %d identity differs from the batch", i)
		}
		if p.TargetTimestamp.IsZero() {
			return fmt.Errorf("point %d target_timestamp is required", i)
		}
		if !p.GeneratedAt.UTC().Equal(first.GeneratedAt.UTC()) {
			return fmt.Errorf("point %d generated_at differs from the batch", i)
		}
	}
	return nil
}

func requireIdentity(tenantID, cuCode, metricName string) error {
	if tenantID == "" || cuCode == "" || metricName == "" {
		return fmt.Errorf("tenant_id, cu_code, and metric_name are required")
	}
	return nil
}
