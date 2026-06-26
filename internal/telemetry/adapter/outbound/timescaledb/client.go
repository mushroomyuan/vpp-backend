package timescaledb

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds TimescaleDB (PostgreSQL) connection parameters.
// Mirrors the style of resource/infrastructure/db/config.go so that the
// telemetry service can reuse the same config file sections and viper
// unmarshalling conventions.
//
// DSN is an optional escape hatch: when non-empty, all structured fields
// (Host, Port, …) are ignored and DSN is forwarded to pgxpool directly.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	// SSLMode is the sslmode parameter (e.g. "disable", "require", "verify-full").
	SSLMode string

	// MaxConns caps the connection pool size.
	// 0 means pgxpool default (typically 4 × GOMAXPROCS).
	MaxConns int32
	// MinConns keeps this many idle connections alive.
	MinConns int32

	// DSN overrides all structured fields above when non-empty.
	DSN string
}

// NewPool creates a *pgxpool.Pool from cfg.
// The pool is configured for high-throughput time-series writes:
//   - pgxpool is used instead of database/sql because it supports
//     pgx.Batch and CopyFrom natively, which are critical for batch inserts.
//   - The caller is responsible for calling pool.Close() at shutdown.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	dsn := cfg.DSN
	if dsn == "" {
		dsn = buildDSN(cfg)
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("timescaledb: parse pool config: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolCfg.MinConns = cfg.MinConns
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("timescaledb: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("timescaledb: ping failed: %w", err)
	}
	return pool, nil
}

// buildDSN assembles a libpq key=value DSN from the structured fields.
func buildDSN(cfg Config) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	parts := []string{
		fmt.Sprintf("host=%s", cfg.Host),
		fmt.Sprintf("port=%d", cfg.Port),
		fmt.Sprintf("user=%s", cfg.User),
		fmt.Sprintf("password=%s", cfg.Password),
		fmt.Sprintf("dbname=%s", cfg.DBName),
		fmt.Sprintf("sslmode=%s", sslMode),
	}
	return strings.Join(parts, " ")
}
