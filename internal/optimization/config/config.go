package config

import (
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/optimization/options"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
)

// Config is the application-level configuration passed through the wiring
// layer (server.go / run.go). CreateFromOptions is the production path
// (cmd → cobra/viper → options → here). Default() is CreateFromOptions
// of options.NewOptions(), kept for tests and as a no-file fallback.
type Config struct {
	HTTPAddr    string
	MetricsAddr string
	ServiceName string

	TelemetryEndpoint string
	TelemetryInsecure bool

	// DecisionInterval must stay strictly greater than Telemetry's
	// collection cycle (Simulator default 30s). See design plan §4.
	DecisionInterval time.Duration
	// DefaultCooldown suppresses repeat (CUCode, RuleID) fires. Zero at
	// the options layer becomes 2× DecisionInterval here.
	DefaultCooldown time.Duration

	// TenantIDs is the set of tenants each tick evaluates. Empty is a
	// valid start-up state (health/metrics still serve; the loop no-ops)
	// until YAML supplies real tenants + rules.
	TenantIDs []string
	Rules     model.Rules

	Telemetry ClientConfig
	Resource  ClientConfig
	Dispatch  ClientConfig
}

// ClientConfig is the outbound gRPC dial shape shared by the three
// upstreams. A nil Breaker is a documented no-op on each client.
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
	interval := opts.Optimization.DecisionInterval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	cooldown := opts.Optimization.DefaultCooldown
	if cooldown <= 0 {
		cooldown = 2 * interval
	}

	return &Config{
		HTTPAddr:          opts.Optimization.HTTPAddr,
		MetricsAddr:       opts.Optimization.MetricsAddr,
		ServiceName:       opts.Optimization.ServiceName,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		DecisionInterval:  interval,
		DefaultCooldown:   cooldown,
		TenantIDs:         append([]string(nil), opts.Optimization.TenantIDs...),
		Rules:             rulesFromOptions(opts.Optimization.Rules),
		Telemetry: ClientConfig{
			Addr:    opts.Telemetry.GRPCAddr,
			Timeout: opts.Telemetry.Timeout,
			Breaker: newBreaker("optimization->telemetry", opts.Telemetry.CircuitBreaker),
		},
		Resource: ClientConfig{
			Addr:    opts.Resource.GRPCAddr,
			Timeout: opts.Resource.Timeout,
			Breaker: newBreaker("optimization->resource", opts.Resource.CircuitBreaker),
		},
		Dispatch: ClientConfig{
			Addr:    opts.Dispatch.GRPCAddr,
			Timeout: opts.Dispatch.Timeout,
			Breaker: newBreaker("optimization->dispatch", opts.Dispatch.CircuitBreaker),
		},
	}
}

func rulesFromOptions(o options.RulesOptions) model.Rules {
	rules := model.DefaultRules()
	if len(o.SOCThresholds) == 0 {
		return rules
	}
	rules.SOCThresholds = make([]model.SOCThresholdRule, 0, len(o.SOCThresholds))
	for _, r := range o.SOCThresholds {
		rules.SOCThresholds = append(rules.SOCThresholds, model.SOCThresholdRule{
			Enabled:          r.Enabled,
			CUCode:           r.CUCode,
			ReadPointKey:     r.ReadPointKey,
			WritePointKey:    r.WritePointKey,
			MinSOC:           r.MinSOC,
			MaxSOC:           r.MaxSOC,
			ChargePowerKW:    r.ChargePowerKW,
			DischargePowerKW: r.DischargePowerKW,
			Cooldown:         r.Cooldown,
		})
	}
	return rules
}

// newBreaker builds a circuit breaker for one outbound gRPC dependency, or
// returns nil when the rule is not enabled. name identifies the dependency
// in logs (e.g. "optimization->telemetry").
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
