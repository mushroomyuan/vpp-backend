package config

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/options"
	"golang.org/x/time/rate"
)

// Config is the application-level configuration passed through the wiring layer.
type Config struct {
	GRPCAddr    string
	MetricsAddr string
	ServiceName string
	ConsulAddr  string

	TelemetryEndpoint string
	TelemetryInsecure bool

	GatewayGRPCAddr string

	Kafka KafkaConfig

	TimeoutScanInterval   time.Duration
	DefaultCommandTimeout time.Duration
	DefaultMaxRetries     int

	TrustProxyHeaders bool
	Authz             AuthzConfig

	RateLimit RateLimitConfig
}

type KafkaConfig struct {
	Brokers       []string
	CommandTopic  string
	DispatchTopic string
	GroupID       string
}

// AuthzConfig wires platform/authz for the dispatch service (C10a).
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

// RateLimitConfig holds ready-to-use limiters resolved from
// options.RateLimitOptions. A nil limiter means "disabled" for that RPC
// (see platform/decorator.WithRateLimiter).
type RateLimitConfig struct {
	SubmitTask *rate.Limiter
	CancelTask *rate.Limiter
}

func CreateFromOptions(opts *options.Options) *Config {
	a := opts.Dispatch.Auth
	az := a.Authz

	authzEnabled := a.TrustProxyHeaders && !az.Disabled
	denyWritesWhenStale := true
	if az.DenyWritesWhenStale != nil {
		denyWritesWhenStale = *az.DenyWritesWhenStale
	}

	return &Config{
		GRPCAddr:              opts.Dispatch.GRPCAddr,
		MetricsAddr:           opts.Dispatch.MetricsAddr,
		ServiceName:           opts.Dispatch.ServiceName,
		ConsulAddr:            opts.Dispatch.ConsulAddr,
		TelemetryEndpoint:     opts.Tracing.Endpoint,
		TelemetryInsecure:     opts.Tracing.Insecure,
		GatewayGRPCAddr:       opts.Gateway.GRPCAddr,
		TimeoutScanInterval:   opts.Dispatch.TimeoutScanInterval,
		DefaultCommandTimeout: opts.Dispatch.DefaultCommandTimeout,
		DefaultMaxRetries:     opts.Dispatch.DefaultMaxRetries,
		Kafka: KafkaConfig{
			Brokers:       opts.Kafka.Brokers,
			CommandTopic:  opts.Kafka.CommandTopic,
			DispatchTopic: opts.Kafka.DispatchTopic,
			GroupID:       opts.Kafka.GroupID,
		},
		TrustProxyHeaders: a.TrustProxyHeaders,
		Authz: AuthzConfig{
			Enabled:              authzEnabled,
			Sync:                 authzEnabled,
			RegisterCatalog:      authzEnabled && !az.DisableRegisterCatalog,
			CasdoorURL:           defaultStr(az.CasdoorURL, "http://127.0.0.1:8000"),
			CasdoorOrg:           defaultStr(az.CasdoorOrg, "built-in"),
			CasdoorApp:           defaultStr(az.CasdoorApp, "app-built-in"),
			CasdoorUsername:      defaultStr(az.CasdoorUsername, "admin"),
			CasdoorPassword:      defaultStr(az.CasdoorPassword, "123"),
			Owner:                defaultStr(az.Owner, "default"),
			ModelFilter:          defaultStr(az.ModelFilter, "default/vpp-rbac"),
			SnapshotPath:         az.SnapshotPath,
			SyncInterval:         parseDuration(az.SyncInterval, 30*time.Second),
			HealthyAfter:         parseDuration(az.HealthyAfter, 1*time.Minute),
			StaleAfter:           parseDuration(az.StaleAfter, 5*time.Minute),
			AllowReadWhenInvalid: az.AllowReadWhenInvalid,
			DenyWritesWhenStale:  denyWritesWhenStale,
		},
		RateLimit: RateLimitConfig{
			SubmitTask: newLimiter(opts.Dispatch.RateLimit.SubmitTask),
			CancelTask: newLimiter(opts.Dispatch.RateLimit.CancelTask),
		},
	}
}

// newLimiter builds a token-bucket limiter from a rule, or returns nil
// (disabled) when the rule is not enabled.
func newLimiter(rule options.RateLimitRule) *rate.Limiter {
	if !rule.Enabled {
		return nil
	}
	return rate.NewLimiter(rate.Limit(rule.RPS), rule.Burst)
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
