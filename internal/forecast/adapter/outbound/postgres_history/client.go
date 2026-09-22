package postgreshistory

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
)

// NewPool creates a *pgxpool.Pool from the same Config shape the rest of
// Forecast uses (config.Config.Postgres). pgxpool is required because
// SaveBatch uses pgx.Batch; platform/postgres's GORM wrapper does not
// expose that API. Caller must Close() the pool on shutdown.
func NewPool(ctx context.Context, cfg platformpostgres.Config) (*pgxpool.Pool, error) {
	dsn := cfg.DSN
	if dsn == "" {
		dsn = buildDSN(cfg)
	}
	if strings.TrimSpace(dsn) == "" || (cfg.DSN == "" && cfg.Host == "") {
		return nil, fmt.Errorf("postgres_history: host or dsn is required")
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres_history: parse pool config: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres_history: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres_history: ping failed: %w", err)
	}
	return pool, nil
}

func buildDSN(cfg platformpostgres.Config) string {
	parts := []string{
		fmt.Sprintf("host=%s", cfg.Host),
		fmt.Sprintf("user=%s", cfg.User),
		fmt.Sprintf("password=%s", cfg.Password),
		fmt.Sprintf("dbname=%s", cfg.DBName),
		fmt.Sprintf("port=%d", cfg.Port),
	}
	for k, v := range cfg.Params {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, " ")
}
