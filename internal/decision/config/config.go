package config

import (
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/mushroomyuan/vpp-backend/decision/options"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
)

// Config is the application-level configuration passed through the wiring
// layer (server.go / run.go). CreateFromOptions is the production path
// (cmd → cobra/viper → options → here).
type Config struct {
	HTTPAddr    string
	GRPCAddr    string
	MetricsAddr string
	ServiceName string

	TelemetryEndpoint string
	TelemetryInsecure bool

	// DecisionInterval must stay strictly greater than Telemetry's
	// collection cycle (Simulator default 30s).
	DecisionInterval time.Duration
	// DefaultCooldown is 2× DecisionInterval when the options value is zero.
	DefaultCooldown time.Duration
	// ScopeCacheTTL is the fresh lifetime of a ResolveScope result.
	ScopeCacheTTL time.Duration
	// ScopeCacheMaxAge is the oldest last-known-good scope used when Resource fails.
	ScopeCacheMaxAge time.Duration
	// TelemetryStaleAge rejects samples older than this relative to the tick.
	TelemetryStaleAge time.Duration
	// ExecutionLease is how long one worker may hold a plan step.
	ExecutionLease time.Duration
	// ExecutionRetryAfter is the delay before an uncertain Dispatch call is retried.
	ExecutionRetryAfter time.Duration
	// ExecutionPollInterval is how often due outbox rows are claimed.
	ExecutionPollInterval time.Duration

	Database DatabaseConfig

	Telemetry ClientConfig
	Resource  ClientConfig
	Dispatch  ClientConfig

	// TrustProxyHeaders requires x-userinfo and a permission check.
	// False bypasses the PEP for local debugging.
	TrustProxyHeaders bool
	Authz             AuthzConfig
}

// AuthzConfig wires platform/authz for the decision service.
type AuthzConfig struct {
	Enabled         bool
	Sync            bool
	RegisterCatalog bool

	CasdoorURL      string
	CasdoorOrg      string
	CasdoorApp      string
	CasdoorUsername string
	CasdoorPassword string

	Owner                string
	ModelFilter          string
	SnapshotPath         string
	SyncInterval         time.Duration
	HealthyAfter         time.Duration
	StaleAfter           time.Duration
	AllowReadWhenInvalid bool
	DenyWritesWhenStale  bool
}

// DatabaseConfig is opened at startup for policy storage.
type DatabaseConfig struct {
	Driver                 string
	Host                   string
	Port                   int
	User                   string
	Password               string
	DBName                 string
	Params                 map[string]string
	DSN                    string
	MaxOpenConns           int
	MaxIdleConns           int
	ConnMaxLifetimeSeconds int
	ConnMaxIdleTimeSeconds int
}

// ClientConfig is the outbound gRPC dial shape shared by the three
// upstreams. A nil Breaker means no breaking. Clients are not dialed yet.
type ClientConfig struct {
	Addr    string
	Timeout time.Duration
	Breaker *gobreaker.CircuitBreaker[any]
}

// Default returns process defaults (no YAML). Equivalent to
// CreateFromOptions(options.NewOptions()).
func Default() *Config {
	return CreateFromOptions(options.NewOptions())
}

// CreateFromOptions maps the YAML/options layer onto ready-to-use values
// (durations already parsed, circuit breakers constructed).
func CreateFromOptions(opts *options.Options) *Config {
	interval := opts.Decision.DecisionInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	cooldown := opts.Decision.DefaultCooldown
	if cooldown <= 0 {
		cooldown = 2 * interval
	}
	scopeTTL := opts.Decision.ScopeCacheTTL
	if scopeTTL <= 0 {
		scopeTTL = 30 * time.Second
	}
	scopeMaxAge := opts.Decision.ScopeCacheMaxAge
	if scopeMaxAge <= 0 {
		scopeMaxAge = 5 * time.Minute
	}
	if scopeMaxAge < scopeTTL {
		scopeMaxAge = scopeTTL
	}
	staleAge := opts.Decision.TelemetryStaleAge
	if staleAge <= 0 {
		staleAge = 90 * time.Second
	}
	lease := opts.Decision.ExecutionLease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	retryAfter := opts.Decision.ExecutionRetryAfter
	if retryAfter <= 0 {
		retryAfter = 5 * time.Second
	}
	poll := opts.Decision.ExecutionPollInterval
	if poll <= 0 {
		poll = time.Second
	}
	authz := authzFromOptions(opts.Auth)

	return &Config{
		HTTPAddr:              opts.Decision.HTTPAddr,
		GRPCAddr:              opts.Decision.GRPCAddr,
		MetricsAddr:           opts.Decision.MetricsAddr,
		ServiceName:           opts.Decision.ServiceName,
		TelemetryEndpoint:     opts.Tracing.Endpoint,
		TelemetryInsecure:     opts.Tracing.Insecure,
		DecisionInterval:      interval,
		DefaultCooldown:       cooldown,
		ScopeCacheTTL:         scopeTTL,
		ScopeCacheMaxAge:      scopeMaxAge,
		TelemetryStaleAge:     staleAge,
		ExecutionLease:        lease,
		ExecutionRetryAfter:   retryAfter,
		ExecutionPollInterval: poll,
		Database:              databaseFromOptions(opts.Database),
		Telemetry: ClientConfig{
			Addr:    opts.Telemetry.GRPCAddr,
			Timeout: opts.Telemetry.Timeout,
			Breaker: newBreaker("decision->telemetry", opts.Telemetry.CircuitBreaker),
		},
		Resource: ClientConfig{
			Addr:    opts.Resource.GRPCAddr,
			Timeout: opts.Resource.Timeout,
			Breaker: newBreaker("decision->resource", opts.Resource.CircuitBreaker),
		},
		Dispatch: ClientConfig{
			Addr:    opts.Dispatch.GRPCAddr,
			Timeout: opts.Dispatch.Timeout,
			Breaker: newBreaker("decision->dispatch", opts.Dispatch.CircuitBreaker),
		},
		TrustProxyHeaders: authz.trust,
		Authz:             authz.cfg,
	}
}

type resolvedAuthz struct {
	trust bool
	cfg   AuthzConfig
}

func authzFromOptions(auth options.AuthOptions) resolvedAuthz {
	az := auth.Authz
	enabled := auth.TrustProxyHeaders && !az.Disabled
	denyWrites := true
	if az.DenyWritesWhenStale != nil {
		denyWrites = *az.DenyWritesWhenStale
	}
	return resolvedAuthz{
		trust: auth.TrustProxyHeaders,
		cfg: AuthzConfig{
			Enabled:              enabled,
			Sync:                 enabled,
			RegisterCatalog:      enabled && !az.DisableRegisterCatalog,
			CasdoorURL:           defaultStr(az.CasdoorURL, "http://127.0.0.1:8000"),
			CasdoorOrg:           defaultStr(az.CasdoorOrg, "built-in"),
			CasdoorApp:           defaultStr(az.CasdoorApp, "app-built-in"),
			CasdoorUsername:      defaultStr(az.CasdoorUsername, "admin"),
			CasdoorPassword:      defaultStr(az.CasdoorPassword, "123"),
			Owner:                defaultStr(az.Owner, "default"),
			ModelFilter:          defaultStr(az.ModelFilter, "default/vpp-rbac"),
			SnapshotPath:         defaultStr(az.SnapshotPath, "./data/decision-authz-snapshot.json"),
			SyncInterval:         parseDuration(az.SyncInterval, 30*time.Second),
			HealthyAfter:         parseDuration(az.HealthyAfter, time.Minute),
			StaleAfter:           parseDuration(az.StaleAfter, 5*time.Minute),
			AllowReadWhenInvalid: az.AllowReadWhenInvalid,
			DenyWritesWhenStale:  denyWrites,
		},
	}
}

func defaultStr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func databaseFromOptions(o options.DatabaseOptions) DatabaseConfig {
	params := make(map[string]string, len(o.Params))
	for k, v := range o.Params {
		params[k] = v
	}
	return DatabaseConfig{
		Driver:                 o.Driver,
		Host:                   o.Host,
		Port:                   o.Port,
		User:                   o.User,
		Password:               o.Password,
		DBName:                 o.DBName,
		Params:                 params,
		DSN:                    o.DSN,
		MaxOpenConns:           o.MaxOpenConns,
		MaxIdleConns:           o.MaxIdleConns,
		ConnMaxLifetimeSeconds: o.ConnMaxLifetimeSeconds,
		ConnMaxIdleTimeSeconds: o.ConnMaxIdleTimeSeconds,
	}
}

// newBreaker builds a circuit breaker for one outbound gRPC dependency, or
// returns nil when the rule is not enabled. name identifies the dependency
// in logs (e.g. "decision->telemetry").
func newBreaker(name string, o options.CircuitBreakerOptions) *gobreaker.CircuitBreaker[any] {
	return resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             o.Enabled,
		Name:                name,
		ConsecutiveFailures: o.ConsecutiveFailures,
		MinRequests:         o.MinRequests,
		FailureRatio:        o.FailureRatio,
		OpenTimeout:         o.OpenTimeout,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})
}
