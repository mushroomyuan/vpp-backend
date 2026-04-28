// Package db provides a driver-agnostic database configuration type shared
// across the infrastructure layer. Concrete infrastructure packages (postgres,
// mysql, …) accept this Config so that callers never need to know which engine
// is in use.
package db

// Config is the driver-agnostic database configuration.
//
// Common relational-database fields (Host, Port, User, Password, DBName) are
// top-level for readability and validation. Driver-specific parameters (e.g.
// sslmode/TimeZone for Postgres, charset/parseTime for MySQL) are carried in
// Params and interpreted by each concrete infrastructure package.
//
// DSN is an optional escape hatch: if non-empty, infrastructure packages must
// use it directly and skip DSN assembly from the structured fields. This
// supports advanced setups (e.g. cloud IAM token DSNs) without breaking the
// common case.
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
