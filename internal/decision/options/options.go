package options

import (
	"fmt"
	"time"
)

// Options holds all configurable parameters for the Decision service.
// Field names and mapstructure tags match config/decision.yaml.
// Policies live in Postgres, not in this file.
type Options struct {
	Decision  DecisionOptions `mapstructure:"decision"`
	Tracing   TracingOptions  `mapstructure:"tracing"`
	Database  DatabaseOptions `mapstructure:"database"`
	Telemetry UpstreamOptions `mapstructure:"telemetry"`
	Resource  UpstreamOptions `mapstructure:"resource"`
	Dispatch  UpstreamOptions `mapstructure:"dispatch"`
	Auth      AuthOptions     `mapstructure:"auth"`
}

// AuthOptions configures gRPC identity middleware.
type AuthOptions struct {
	// TrustProxyHeaders requires valid x-userinfo when true.
	// When false, auth is bypassed for local direct debugging.
	TrustProxyHeaders bool         `mapstructure:"trust-proxy-headers"`
	Authz             AuthzOptions `mapstructure:"authz"`
}

// AuthzOptions wires the local permission checker and Casdoor sync.
type AuthzOptions struct {
	Disabled bool `mapstructure:"disabled"`

	CasdoorURL      string `mapstructure:"casdoor-url"`
	CasdoorOrg      string `mapstructure:"casdoor-organization"`
	CasdoorApp      string `mapstructure:"casdoor-application"`
	CasdoorUsername string `mapstructure:"casdoor-username"`
	CasdoorPassword string `mapstructure:"casdoor-password"`

	Owner        string `mapstructure:"owner"`
	ModelFilter  string `mapstructure:"model-filter"`
	SnapshotPath string `mapstructure:"snapshot-path"`

	SyncInterval         string `mapstructure:"sync-interval"`
	HealthyAfter         string `mapstructure:"healthy-after"`
	StaleAfter           string `mapstructure:"stale-after"`
	AllowReadWhenInvalid bool   `mapstructure:"allow-read-when-invalid"`
	// DenyWritesWhenStale defaults true when the key is omitted.
	DenyWritesWhenStale *bool `mapstructure:"deny-writes-when-stale"`

	DisableRegisterCatalog bool `mapstructure:"disable-register-catalog"`
}

type DecisionOptions struct {
	HTTPAddr    string `mapstructure:"http-addr"`
	GRPCAddr    string `mapstructure:"grpc-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`

	// DecisionInterval must stay strictly greater than Telemetry's
	// collection cycle (Simulator default 30s).
	DecisionInterval time.Duration `mapstructure:"decision-interval"`
	// DefaultCooldown is the fallback when a policy does not set its own.
	// Zero means "use 2× DecisionInterval".
	DefaultCooldown time.Duration `mapstructure:"default-cooldown"`
	// ScopeCacheTTL is how long a successful ResolveScope is reused.
	// Zero means 30s. Enable and update do not use this cache.
	ScopeCacheTTL time.Duration `mapstructure:"scope-cache-ttl"`
	// ScopeCacheMaxAge is how long that copy may be used after Resource fails.
	// Zero means 5m, raised to the TTL when the TTL is longer.
	ScopeCacheMaxAge time.Duration `mapstructure:"scope-cache-max-age"`
	// TelemetryStaleAge is the per-metric freshness limit passed to GetSnapshots.
	// Zero means 90s.
	TelemetryStaleAge time.Duration `mapstructure:"telemetry-stale-age"`
	// ExecutionLease is how long one worker may hold a claimed plan step.
	// Zero means 30s.
	ExecutionLease time.Duration `mapstructure:"execution-lease"`
	// ExecutionRetryAfter delays the next claim after an uncertain Dispatch call.
	// Zero means 5s. The idempotency key does not change.
	ExecutionRetryAfter time.Duration `mapstructure:"execution-retry-after"`
	// ExecutionPollInterval is how often the execution loop looks for due outbox rows.
	// Zero means 1s.
	ExecutionPollInterval time.Duration `mapstructure:"execution-poll-interval"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

// DatabaseOptions is parsed now and opened in a later slice.
// DSN, when set, replaces the structured host/user/dbname fields.
type DatabaseOptions struct {
	Driver   string            `mapstructure:"driver"`
	Host     string            `mapstructure:"host"`
	Port     int               `mapstructure:"port"`
	User     string            `mapstructure:"user"`
	Password string            `mapstructure:"password"`
	DBName   string            `mapstructure:"dbname"`
	Params   map[string]string `mapstructure:"params"`
	DSN      string            `mapstructure:"dsn"`

	MaxOpenConns           int `mapstructure:"max-open-conns"`
	MaxIdleConns           int `mapstructure:"max-idle-conns"`
	ConnMaxLifetimeSeconds int `mapstructure:"conn-max-lifetime-seconds"`
	ConnMaxIdleTimeSeconds int `mapstructure:"conn-max-idle-time-seconds"`
}

// UpstreamOptions is the outbound gRPC dial + breaker shape shared by
// telemetry / resource / dispatch. Clients are constructed in a later slice.
type UpstreamOptions struct {
	GRPCAddr       string                `mapstructure:"grpc-addr"`
	Timeout        time.Duration         `mapstructure:"timeout"`
	CircuitBreaker CircuitBreakerOptions `mapstructure:"circuit-breaker"`
}

// CircuitBreakerOptions configures a platform/resilience circuit breaker
// for one outbound dependency. Enabled=false means no breaking.
type CircuitBreakerOptions struct {
	Enabled             bool          `mapstructure:"enabled"`
	ConsecutiveFailures uint32        `mapstructure:"consecutive-failures"`
	MinRequests         uint32        `mapstructure:"min-requests"`
	FailureRatio        float64       `mapstructure:"failure-ratio"`
	OpenTimeout         time.Duration `mapstructure:"open-timeout"`
}

func NewOptions() *Options {
	interval := 60 * time.Second
	return &Options{
		Decision: DecisionOptions{
			HTTPAddr:         "127.0.0.1:8088",
			GRPCAddr:         ":5008",
			MetricsAddr:      ":9108",
			ServiceName:      "vpp-decision",
			DecisionInterval: interval,
			DefaultCooldown:  0,
		},
		Database: DatabaseOptions{
			Driver:                 "postgres",
			Host:                   "127.0.0.1",
			Port:                   5432,
			User:                   "postgres",
			Password:               "postgres123",
			DBName:                 "decision",
			Params:                 map[string]string{"sslmode": "disable", "TimeZone": "Asia/Shanghai"},
			MaxOpenConns:           50,
			MaxIdleConns:           10,
			ConnMaxLifetimeSeconds: 1800,
			ConnMaxIdleTimeSeconds: 300,
		},
		Telemetry: UpstreamOptions{
			GRPCAddr:       "127.0.0.1:5003",
			Timeout:        5 * time.Second,
			CircuitBreaker: defaultBreaker(),
		},
		Resource: UpstreamOptions{
			GRPCAddr:       "127.0.0.1:5002",
			Timeout:        5 * time.Second,
			CircuitBreaker: defaultBreaker(),
		},
		Dispatch: UpstreamOptions{
			GRPCAddr:       "127.0.0.1:5006",
			Timeout:        5 * time.Second,
			CircuitBreaker: defaultBreaker(),
		},
	}
}

func defaultBreaker() CircuitBreakerOptions {
	return CircuitBreakerOptions{
		Enabled:             true,
		ConsecutiveFailures: 5,
		MinRequests:         10,
		FailureRatio:        0.5,
		OpenTimeout:         30 * time.Second,
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Decision.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("decision.http-addr must not be empty"))
	}
	if o.Decision.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("decision.grpc-addr must not be empty"))
	}
	if o.Decision.MetricsAddr == "" {
		errs = append(errs, fmt.Errorf("decision.metrics-addr must not be empty"))
	}
	if o.Decision.ServiceName == "" {
		errs = append(errs, fmt.Errorf("decision.service-name must not be empty"))
	}
	if o.Decision.DecisionInterval <= 30*time.Second {
		errs = append(errs, fmt.Errorf("decision.decision-interval must be greater than 30s (Telemetry collection cycle)"))
	}
	if o.Decision.ScopeCacheTTL < 0 {
		errs = append(errs, fmt.Errorf("decision.scope-cache-ttl must be zero or positive"))
	}
	if o.Decision.ScopeCacheMaxAge < 0 {
		errs = append(errs, fmt.Errorf("decision.scope-cache-max-age must be zero or positive"))
	}
	if o.Decision.ScopeCacheTTL > 0 && o.Decision.ScopeCacheMaxAge > 0 && o.Decision.ScopeCacheMaxAge < o.Decision.ScopeCacheTTL {
		errs = append(errs, fmt.Errorf("decision.scope-cache-max-age must be >= scope-cache-ttl"))
	}
	if o.Decision.TelemetryStaleAge < 0 {
		errs = append(errs, fmt.Errorf("decision.telemetry-stale-age must be zero or positive"))
	}
	if o.Decision.ExecutionLease < 0 || o.Decision.ExecutionRetryAfter < 0 || o.Decision.ExecutionPollInterval < 0 {
		errs = append(errs, fmt.Errorf("decision execution lease, retry, and poll interval must be zero or positive"))
	}
	if o.Database.Driver == "" {
		errs = append(errs, fmt.Errorf("database.driver must not be empty"))
	}
	if o.Database.DSN == "" {
		if o.Database.Host == "" {
			errs = append(errs, fmt.Errorf("database.host must not be empty (or set database.dsn)"))
		}
		if o.Database.User == "" {
			errs = append(errs, fmt.Errorf("database.user must not be empty (or set database.dsn)"))
		}
		if o.Database.DBName == "" {
			errs = append(errs, fmt.Errorf("database.dbname must not be empty (or set database.dsn)"))
		}
	}
	if o.Telemetry.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("telemetry.grpc-addr must not be empty"))
	}
	if o.Resource.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("resource.grpc-addr must not be empty"))
	}
	if o.Dispatch.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("dispatch.grpc-addr must not be empty"))
	}
	return errs
}
