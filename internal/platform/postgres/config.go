// Package postgres provides a shared PostgreSQL connection factory and
// driver-agnostic configuration used by all GORM-based services.
//
// Usage mirrors platform/redis: import the package, pass a Config to
// NewPostgres, and inject the returned *Postgres into repository layers.
package postgres

// Config is the driver-agnostic database configuration shared across services.
//
// Common relational-database fields are explicit for readability and
// validation. Driver-specific parameters (e.g. sslmode/TimeZone for Postgres,
// charset/parseTime for MySQL) are carried in Params and forwarded verbatim
// to the DSN builder.
//
// DSN is an optional escape hatch: if non-empty, it is used directly and all
// structured fields are ignored. This supports advanced setups such as cloud
// IAM token DSNs without breaking the common case.
type Config struct {
	Driver   string
	Host     string
	Port     int
	User     string
	Password string
	DBName   string

	// Params holds driver-specific connection parameters.
	// Postgres examples: {"sslmode": "disable", "TimeZone": "Asia/Shanghai"}
	// MySQL examples:    {"charset": "utf8mb4", "parseTime": "True", "loc": "Local"}
	Params map[string]string

	// DSN overrides all structured fields above when non-empty.
	DSN string

	MaxOpenConns           int
	MaxIdleConns           int
	ConnMaxLifetimeSeconds int
	ConnMaxIdleTimeSeconds int
}
