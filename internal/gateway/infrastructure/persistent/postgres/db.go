package postgres

import (
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	"gorm.io/gorm"
)

// Postgres is a type alias for the shared platform Postgres wrapper.
// All repository structs in this package hold a *Postgres without change.
type Postgres = platformpostgres.Postgres

// NewPostgres opens a PostgreSQL connection pool. Delegates entirely to the
// platform implementation; no service-specific logic lives here.
func NewPostgres(cfg platformpostgres.Config) *Postgres {
	return platformpostgres.NewPostgres(cfg)
}

// NewPostgresWithDB wraps an existing *gorm.DB for test injection.
func NewPostgresWithDB(db *gorm.DB) *Postgres {
	return platformpostgres.NewPostgresWithDB(db)
}
