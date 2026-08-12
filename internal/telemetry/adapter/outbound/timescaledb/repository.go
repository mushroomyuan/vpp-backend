package timescaledb

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// NewStores returns a TelemetryStore and an AggregationStore that share pool.
// Prefer creating both via this constructor so the pool is not duplicated.
func NewStores(pool *pgxpool.Pool) (*TelemetryStore, *AggregationStore) {
	return &TelemetryStore{pool: pool}, &AggregationStore{pool: pool}
}

// ── TelemetryStore  (port.TelemetryRepository) ───────────────────────────────

// TelemetryStore writes and reads raw telemetry records from TimescaleDB.
type TelemetryStore struct {
	pool *pgxpool.Pool
}

const insertSQL = `
INSERT INTO telemetry_records (ts, tenant_id, cu_code, metric_name, metric_type, value)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ts, tenant_id, cu_code, metric_name) DO NOTHING`

// SaveBatch writes all records to TimescaleDB using pgx.Batch — a single
// network round-trip regardless of how many rows are sent.
// QualityBad / QualityUncertain metrics are silently skipped (filtered by
// recordsToInsertRows in mapper.go).
func (s *TelemetryStore) SaveBatch(ctx context.Context, records []*model.TelemetryRecord) error {
	rows := recordsToInsertRows(records)
	if len(rows) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, r := range rows {
		batch.Queue(insertSQL, r.ts, r.tenantID, r.cuCode, r.metricName, r.metricType, r.value)
	}

	results := s.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	for range rows {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("timescaledb SaveBatch: %w", err)
		}
	}
	return nil
}

const querySQL = `
SELECT ts, tenant_id, cu_code, metric_name, metric_type, value
FROM   telemetry_records
WHERE  tenant_id = $1
  AND  cu_code   = $2
  AND  ts >= $3
  AND  ts <  $4
%s
ORDER BY ts ASC, metric_name ASC`

// Query returns raw TelemetryRecords matching the condition.
// Rows are grouped by (timestamp × cu_code) to reconstruct the original
// TelemetryRecord envelopes (one per CU per timestamp, carrying N metrics).
func (s *TelemetryStore) Query(
	ctx context.Context,
	condition model.QueryCondition,
) ([]*model.TelemetryRecord, error) {
	if err := condition.Validate(); err != nil {
		return nil, err
	}

	var metricFilter string
	args := []interface{}{condition.TenantID, condition.CUCode, condition.StartTime, condition.EndTime}
	if condition.MetricName != "" {
		metricFilter = "AND metric_name = $5"
		args = append(args, condition.MetricName)
	}

	sql := fmt.Sprintf(querySQL, metricFilter)
	pgxRows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("timescaledb Query: %w", err)
	}
	defer pgxRows.Close()

	var rawRows []rawRow
	for pgxRows.Next() {
		var r rawRow
		if err := pgxRows.Scan(&r.ts, &r.tenantID, &r.cuCode, &r.metricName, &r.metricType, &r.value); err != nil {
			return nil, fmt.Errorf("timescaledb Query scan: %w", err)
		}
		rawRows = append(rawRows, r)
	}
	if err := pgxRows.Err(); err != nil {
		return nil, fmt.Errorf("timescaledb Query rows: %w", err)
	}
	return rawRowsToRecords(rawRows), nil
}

// ── AggregationStore  (port.AggregationRepository) ───────────────────────────

// AggregationStore executes downsampled aggregation queries against TimescaleDB.
// It uses time_bucket() with a parameterized interval so the step is pushed
// down to the storage engine — no post-processing in Go.
type AggregationStore struct {
	pool *pgxpool.Pool
}

func (s *AggregationStore) Query(
	ctx context.Context,
	q model.AggregationQuery,
) ([]*model.AggregatedPoint, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}

	requested := make(map[model.AggFunction]bool, len(q.Functions))
	for _, fn := range q.Functions {
		requested[fn] = true
	}

	sql := buildAggSQL(requested)
	stepStr := pgIntervalString(q.Step)
	pgxRows, err := s.pool.Query(ctx, sql,
		stepStr, q.TenantID, q.CUCode, q.MetricName, q.StartTime, q.EndTime,
	)
	if err != nil {
		return nil, fmt.Errorf("timescaledb AggQuery: %w", err)
	}
	defer pgxRows.Close()

	var points []*model.AggregatedPoint
	for pgxRows.Next() {
		var (
			bucket     time.Time
			bucketEnd  time.Time
			tenantID   string
			cuCode     string
			metricName string
		)

		p := &model.AggregatedPoint{}

		// Build the scan destination list: fixed columns first, then the
		// requested aggregation columns in the same order as buildAggSQL.
		dests := []interface{}{&bucket, &bucketEnd, &tenantID, &cuCode, &metricName}
		if requested[model.AggAvg] {
			dests = append(dests, &p.Avg)
		}
		if requested[model.AggMax] {
			dests = append(dests, &p.Max)
		}
		if requested[model.AggMin] {
			dests = append(dests, &p.Min)
		}
		if requested[model.AggSum] {
			dests = append(dests, &p.Sum)
		}
		if requested[model.AggCount] {
			dests = append(dests, &p.Count)
		}
		if requested[model.AggLast] {
			dests = append(dests, &p.Last)
		}

		if err := pgxRows.Scan(dests...); err != nil {
			return nil, fmt.Errorf("timescaledb AggQuery scan: %w", err)
		}
		p.CUCode = cuCode
		p.MetricName = metricName
		p.StartTime = bucket
		p.EndTime = bucketEnd
		points = append(points, p)
	}
	if err := pgxRows.Err(); err != nil {
		return nil, fmt.Errorf("timescaledb AggQuery rows: %w", err)
	}
	return points, nil
}

// pgIntervalString converts a time.Duration to a PostgreSQL interval literal
// that can be cast via $1::interval inside a Flux query.
// e.g. 15*time.Minute → "15 minutes", 1*time.Hour → "1 hours"
func pgIntervalString(d time.Duration) string {
	switch {
	case d >= 24*time.Hour && d%(24*time.Hour) == 0:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	case d >= time.Hour && d%time.Hour == 0:
		return fmt.Sprintf("%d hours", int(d.Hours()))
	case d >= time.Minute && d%time.Minute == 0:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
}

// ── compile-time interface assertions ─────────────────────────────────────────

var _ port.TelemetryRepository = (*TelemetryStore)(nil)
var _ port.AggregationRepository = (*AggregationStore)(nil)
