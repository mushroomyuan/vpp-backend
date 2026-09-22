package config

import (
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/options"
	platformpostgres "github.com/mushroomyuan/vpp-backend/platform/postgres"
	platformredis "github.com/mushroomyuan/vpp-backend/platform/redis"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
)

// Config is the application-level configuration passed through the wiring
// layer (server.go / run.go). CreateFromOptions is the production path
// (cmd → cobra/viper → options → here). Default() is CreateFromOptions
// of options.NewOptions(), kept for tests and as a no-file fallback.
type Config struct {
	GRPCAddr    string
	HTTPAddr    string
	MetricsAddr string
	ServiceName string

	TelemetryEndpoint string
	TelemetryInsecure bool

	CycleInterval time.Duration
	HorizonSteps  int
	StepSeconds   int64
	HistoryWindow time.Duration
	// RedisTTL = HorizonSteps×StepSeconds + CycleInterval (design plan §7.1).
	RedisTTL time.Duration

	Targets []model.ForecastTarget

	Telemetry ClientConfig
	Postgres  platformpostgres.Config
	Redis     platformredis.Config
}

// ClientConfig is the outbound gRPC dial shape for Telemetry.
// A nil Breaker is a documented no-op on the client.
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
// (durations already parsed, circuit breaker constructed, Redis TTL derived).
func CreateFromOptions(opts *options.Options) *Config {
	interval := opts.Forecast.CycleInterval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	horizon := opts.Forecast.HorizonSteps
	if horizon < 1 {
		horizon = 4
	}
	step := opts.Forecast.StepSeconds
	if step <= 0 {
		step = 900
	}
	history := opts.Forecast.HistoryWindow
	if history <= 0 {
		history = 168 * time.Hour
	}

	return &Config{
		GRPCAddr:          opts.Forecast.GRPCAddr,
		HTTPAddr:          opts.Forecast.HTTPAddr,
		MetricsAddr:       opts.Forecast.MetricsAddr,
		ServiceName:       opts.Forecast.ServiceName,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		CycleInterval:     interval,
		HorizonSteps:      horizon,
		StepSeconds:       step,
		HistoryWindow:     history,
		RedisTTL:          time.Duration(horizon)*time.Duration(step)*time.Second + interval,
		Targets:           targetsFromOptions(opts.Forecast.Targets),
		Telemetry: ClientConfig{
			Addr:    opts.Telemetry.GRPCAddr,
			Timeout: opts.Telemetry.Timeout,
			Breaker: newBreaker("forecast->telemetry", opts.Telemetry.CircuitBreaker),
		},
		Postgres: postgresFromOptions(opts.Postgres),
		Redis:    redisFromOptions(opts.Redis),
	}
}

func targetsFromOptions(in []options.TargetOptions) []model.ForecastTarget {
	out := make([]model.ForecastTarget, 0, len(in))
	for _, t := range in {
		out = append(out, model.ForecastTarget{
			Enabled:                t.Enabled,
			TenantID:               t.TenantID,
			CUCode:                 t.CUCode,
			MetricName:             t.MetricName,
			Algorithm:              model.AlgorithmID(t.Algorithm),
			MovingAverageWindow:    t.MovingAverageWindow,
			SamePeriodLookbackDays: t.SamePeriodLookbackDays,
		})
	}
	return out
}

func postgresFromOptions(o options.DatabaseOptions) platformpostgres.Config {
	params := make(map[string]string, len(o.Params))
	for k, v := range o.Params {
		params[k] = v
	}
	return platformpostgres.Config{
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

func redisFromOptions(o options.RedisOptions) platformredis.Config {
	return platformredis.Config{
		Addr:                o.Addr,
		Password:            o.Password,
		DB:                  o.DB,
		PoolSize:            o.PoolSize,
		MinIdleConns:        o.MinIdleConns,
		DialTimeoutSeconds:  o.DialTimeoutSeconds,
		ReadTimeoutSeconds:  o.ReadTimeoutSeconds,
		WriteTimeoutSeconds: o.WriteTimeoutSeconds,
		PingTimeoutSeconds:  o.PingTimeoutSeconds,
	}
}

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
