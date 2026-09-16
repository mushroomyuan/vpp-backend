package options

import (
	"fmt"
	"time"
)

// Options holds all configurable parameters for the Optimization service.
// Field names and mapstructure tags match config/optimization.yaml.
type Options struct {
	Optimization OptimizationOptions `mapstructure:"optimization"`
	Tracing      TracingOptions      `mapstructure:"tracing"`
	Telemetry    UpstreamOptions     `mapstructure:"telemetry"`
	Resource     UpstreamOptions     `mapstructure:"resource"`
	Dispatch     UpstreamOptions     `mapstructure:"dispatch"`
}

type OptimizationOptions struct {
	HTTPAddr    string `mapstructure:"http-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`

	// DecisionInterval must stay strictly greater than Telemetry's
	// collection cycle (Simulator default 30s). See design plan §4.
	DecisionInterval time.Duration `mapstructure:"decision-interval"`
	// DefaultCooldown suppresses repeat (CUCode, RuleID) fires. Zero means
	// "use 2× DecisionInterval" rather than "no cooldown".
	DefaultCooldown time.Duration `mapstructure:"default-cooldown"`

	TenantIDs []string     `mapstructure:"tenant-ids"`
	Rules     RulesOptions `mapstructure:"rules"`
}

// RulesOptions is the YAML shape of v1's policy set. Empty is valid: the
// process still serves /healthz and /metrics, and the decision loop no-ops
// until a deployment fills this in. Domain DefaultRules() stays empty for
// the same reason — concrete CU/threshold values are not hardcoded.
type RulesOptions struct {
	SOCThresholds []SOCThresholdRuleOptions `mapstructure:"soc-thresholds"`
}

type SOCThresholdRuleOptions struct {
	Enabled          bool          `mapstructure:"enabled"`
	CUCode           string        `mapstructure:"cu-code"`
	ReadPointKey     string        `mapstructure:"read-point-key"`
	WritePointKey    string        `mapstructure:"write-point-key"`
	MinSOC           float64       `mapstructure:"min-soc"`
	MaxSOC           float64       `mapstructure:"max-soc"`
	ChargePowerKW    float64       `mapstructure:"charge-power-kw"`
	DischargePowerKW float64       `mapstructure:"discharge-power-kw"`
	Cooldown         time.Duration `mapstructure:"cooldown"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

// UpstreamOptions is the outbound gRPC dial + breaker shape shared by
// telemetry / resource / dispatch. Optimization is the first high-frequency
// polling client, so CircuitBreaker defaults to enabled (design plan §9).
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
		Optimization: OptimizationOptions{
			HTTPAddr:         ":8088",
			MetricsAddr:      ":9108",
			ServiceName:      "vpp-optimization",
			DecisionInterval: interval,
			// 0 → CreateFromOptions derives 2× DecisionInterval.
			DefaultCooldown: 0,
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
	if o.Optimization.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("optimization.http-addr must not be empty"))
	}
	if o.Optimization.MetricsAddr == "" {
		errs = append(errs, fmt.Errorf("optimization.metrics-addr must not be empty"))
	}
	if o.Optimization.ServiceName == "" {
		errs = append(errs, fmt.Errorf("optimization.service-name must not be empty"))
	}
	if o.Optimization.DecisionInterval <= 30*time.Second {
		errs = append(errs, fmt.Errorf("optimization.decision-interval must be greater than 30s (Telemetry collection cycle)"))
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
	for i, r := range o.Optimization.Rules.SOCThresholds {
		if !r.Enabled {
			continue
		}
		prefix := fmt.Sprintf("optimization.rules.soc-thresholds[%d]", i)
		if r.CUCode == "" {
			errs = append(errs, fmt.Errorf("%s.cu-code must not be empty", prefix))
		}
		if r.ReadPointKey == "" {
			errs = append(errs, fmt.Errorf("%s.read-point-key must not be empty", prefix))
		}
		if r.WritePointKey == "" {
			errs = append(errs, fmt.Errorf("%s.write-point-key must not be empty", prefix))
		}
		if r.MinSOC >= r.MaxSOC {
			errs = append(errs, fmt.Errorf("%s.min-soc (%v) must be less than max-soc (%v)", prefix, r.MinSOC, r.MaxSOC))
		}
	}
	return errs
}
