// Package postgreshistory implements domain/port.HistoryPort against
// the forecast_history table using pgx.Batch, matching telemetry's
// TimescaleDB SaveBatch (design plan §7.2). Postgres is the
// authoritative store; Redis is only a hot cache of the latest batch.
package postgreshistory
