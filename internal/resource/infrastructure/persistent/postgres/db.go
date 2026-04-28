package postgres

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	infradb "github.com/mushroomyuan/vpp-backend/resource/infrastructure/db"
	"github.com/sirupsen/logrus"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Postgres wraps a *gorm.DB and exposes helper methods used by the repository
// implementations in this package.
type Postgres struct {
	db *gorm.DB
}

// NewPostgres opens a PostgreSQL connection pool from cfg.
// If cfg.DSN is non-empty it is used directly (escape hatch); otherwise the
// DSN is assembled from the structured fields and cfg.Params.
func NewPostgres(cfg infradb.Config) *Postgres {
	db, err := gorm.Open(gormpg.Open(buildDSN(cfg)), &gorm.Config{})
	if err != nil {
		logrus.Panicf("connect to postgres failed: %v", err)
	}
	applyPoolSettings(db, cfg)
	return &Postgres{db: db}
}

// NewPostgresWithDB is used in tests to inject a pre-configured *gorm.DB.
func NewPostgresWithDB(db *gorm.DB) *Postgres {
	return &Postgres{db: db}
}

func (p *Postgres) DB() *gorm.DB { return p.db }

// SQLDb returns the underlying *sql.DB for use with external tools such as
// connection-pool metrics collectors. Callers must not close the returned
// *sql.DB directly; its lifetime is managed by this Postgres instance.
func (p *Postgres) SQLDb() (*sql.DB, error) {
	return p.db.DB()
}

// UseTransaction returns tx if non-nil, otherwise falls back to the pool DB.
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

// buildDSN constructs a Postgres key=value DSN from cfg.
// If cfg.DSN is already set it is returned as-is (escape hatch for complex
// setups like IAM-token connections).
// Otherwise the common fields (host, user, password, dbname, port) are written
// first, then every entry in cfg.Params is appended, letting callers supply
// postgres-specific options such as sslmode and TimeZone without this package
// needing to know about them.
func buildDSN(cfg infradb.Config) string {
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

func applyPoolSettings(db *gorm.DB, cfg infradb.Config) {
	sqlDB, err := db.DB()
	if err != nil {
		logrus.Warnf("get postgres sql.DB failed: %v", err)
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
