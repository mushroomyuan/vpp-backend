package timescaledb

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ApplySchema creates the telemetry hypertable and the 15-minute continuous
// aggregate if they do not yet exist. Safe to call on every service startup
// (all statements are idempotent via IF NOT EXISTS / if_not_exists => TRUE).
//
// DDL overview:
//
//   telemetry_records  — narrow-table hypertable (one row per metric sample)
//   telemetry_15m      — continuous aggregate; bucket = 15 minutes
//
// The continuous aggregate covers AVG / MAX / MIN / SUM / COUNT / LAST for
// every (tenant_id, cu_code, metric_name) group and is refreshed automatically
// by TimescaleDB's background worker within a 5-minute schedule.
func ApplySchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []struct {
		name string
		sql  string
	}{
		{
			"create telemetry_records table",
			`CREATE TABLE IF NOT EXISTS telemetry_records (
				ts          TIMESTAMPTZ      NOT NULL,
				tenant_id   TEXT             NOT NULL,
				cu_code     TEXT             NOT NULL,
				metric_name TEXT             NOT NULL,
				metric_type TEXT             NOT NULL,
				value       DOUBLE PRECISION NOT NULL,
				CONSTRAINT telemetry_records_pkey
					PRIMARY KEY (ts, tenant_id, cu_code, metric_name)
			)`,
		},
		{
			"convert to hypertable",
			// chunk_time_interval = 1 day is a good default for 400 points/sec.
			// Increase to 1 week for lower-frequency deployments.
			`SELECT create_hypertable(
				'telemetry_records', 'ts',
				chunk_time_interval => INTERVAL '1 day',
				if_not_exists => TRUE
			)`,
		},
		{
			"create retention index on cu_code",
			// Speeds up the most common query pattern: tenant + CU + time range.
			`CREATE INDEX IF NOT EXISTS idx_telemetry_cu
				ON telemetry_records (tenant_id, cu_code, ts DESC)`,
		},
		{
			"create continuous aggregate telemetry_15m",
			// last(value, ts) is a TimescaleDB built-in aggregate that returns
			// the value corresponding to the maximum ts in the group.
			`CREATE MATERIALIZED VIEW IF NOT EXISTS telemetry_15m
			WITH (timescaledb.continuous) AS
			SELECT
				time_bucket('15 minutes', ts)  AS bucket,
				tenant_id,
				cu_code,
				metric_name,
				AVG(value)           AS avg,
				MAX(value)           AS max,
				MIN(value)           AS min,
				SUM(value)           AS sum,
				COUNT(value)::bigint AS count,
				last(value, ts)      AS last
			FROM telemetry_records
			GROUP BY 1, 2, 3, 4
			WITH NO DATA`,
		},
		{
			"add continuous aggregate refresh policy",
			// Keeps telemetry_15m up-to-date: refresh everything older than
			// 1 minute up to 1 day in the past, every 5 minutes.
			`SELECT add_continuous_aggregate_policy('telemetry_15m',
				start_offset  => INTERVAL '1 day',
				end_offset    => INTERVAL '1 minute',
				schedule_interval => INTERVAL '5 minutes',
				if_not_exists => TRUE
			)`,
		},
	}

	for _, s := range statements {
		if _, err := pool.Exec(ctx, s.sql); err != nil {
			return fmt.Errorf("timescaledb schema [%s]: %w", s.name, err)
		}
	}
	return nil
}
