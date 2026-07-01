package postgres

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Postgres wraps a *gorm.DB and exposes helper methods used by repository
// implementations. It is the shared connection factory for all GORM-based
// services, mirroring the platform/redis.Client pattern.
type Postgres struct {
	db *gorm.DB
}

// NewPostgres opens a PostgreSQL connection pool from cfg.
// If cfg.DSN is non-empty it is used directly; otherwise the DSN is assembled
// from the structured fields and cfg.Params.
// Panics on connection failure — callers (server.go composition roots) are
// expected to treat startup errors as fatal.
func NewPostgres(cfg Config) *Postgres {
	db, err := gorm.Open(gormpg.Open(buildDSN(cfg)), &gorm.Config{})
	if err != nil {
		logrus.Panicf("platform/postgres: connect failed: %v", err)
	}
	applyPoolSettings(db, cfg)
	return &Postgres{db: db}
}

// NewPostgresWithDB wraps an existing *gorm.DB. Intended for tests that inject
// a pre-configured or in-memory database without going through NewPostgres.
func NewPostgresWithDB(db *gorm.DB) *Postgres {
	return &Postgres{db: db}
}

// DB returns the underlying *gorm.DB for use by repository methods.
func (p *Postgres) DB() *gorm.DB { return p.db }

// SQLDb returns the underlying *sql.DB for use with external tools such as
// connection-pool metrics collectors. The caller must not close the returned
// *sql.DB; its lifetime is managed by this Postgres instance.
func (p *Postgres) SQLDb() (*sql.DB, error) {
	return p.db.DB()
}

// UseTransaction returns tx when non-nil, falling back to the pool DB.
// Repository methods call this to support both transactional and non-transactional
// execution without separate code paths.
func (p *Postgres) UseTransaction(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		return p.db
	}
	return tx
}

// StartTransaction runs f inside a single database transaction.
func (p *Postgres) StartTransaction(f func(tx *gorm.DB) error) error {
	return p.db.Transaction(f)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func buildDSN(cfg Config) string {
	if cfg.DSN != "" {
		return cfg.DSN
	}
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

func applyPoolSettings(db *gorm.DB, cfg Config) {
	sqlDB, err := db.DB()
	if err != nil {
		logrus.Warnf("platform/postgres: get sql.DB failed: %v", err)
		return
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetimeSeconds > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSeconds) * time.Second)
	}
	if cfg.ConnMaxIdleTimeSeconds > 0 {
		sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeSeconds) * time.Second)
	}
}
