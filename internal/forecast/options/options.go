package options

import (
	"fmt"
	"time"
)

// Options holds all configurable parameters for the Forecast service.
// Field names and mapstructure tags match config/forecast.yaml.
type Options struct {
	Forecast  ForecastOptions  `mapstructure:"forecast"`
	Tracing   TracingOptions   `mapstructure:"tracing"`
	Telemetry UpstreamOptions  `mapstructure:"telemetry"`
	Postgres  DatabaseOptions  `mapstructure:"postgres"`
	Redis     RedisOptions     `mapstructure:"redis"`
}

type ForecastOptions struct {
	GRPCAddr    string `mapstructure:"grpc-addr"`
	HTTPAddr    string `mapstructure:"http-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`

	// CycleInterval is how often the batch loop ticks. Unlike Optimization
	// this is not a correctness constraint (no side-effecting commands), so
	// it is not forced above Telemetry's 30s collection cycle.
	CycleInterval time.Duration `mapstructure:"cycle-interval"`
	HorizonSteps  int           `mapstructure:"horizon-steps"`
	StepSeconds   int64         `mapstructure:"step-seconds"`
	HistoryWindow time.Duration `mapstructure:"history-window"`

	Targets []TargetOptions `mapstructure:"targets"`
}

// TargetOptions is the YAML shape of one model.ForecastTarget.
type TargetOptions struct {
	Enabled    bool   `mapstructure:"enabled"`
	TenantID   string `mapstructure:"tenant-id"`
	CUCode     string `mapstructure:"cu-code"`
	MetricName string `mapstructure:"metric-name"`
	Algorithm  string `mapstructure:"algorithm"`

	MovingAverageWindow    int `mapstructure:"moving-average-window"`
	SamePeriodLookbackDays int `mapstructure:"same-period-lookback-days"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

// UpstreamOptions is the outbound gRPC dial + breaker shape for Telemetry.
type UpstreamOptions struct {
	GRPCAddr       string                `mapstructure:"grpc-addr"`
	Timeout        time.Duration         `mapstructure:"timeout"`
	CircuitBreaker CircuitBreakerOptions `mapstructure:"circuit-breaker"`
}

type CircuitBreakerOptions struct {
	Enabled             bool          `mapstructure:"enabled"`
	ConsecutiveFailures uint32        `mapstructure:"consecutive-failures"`
	MinRequests         uint32        `mapstructure:"min-requests"`
	FailureRatio        float64       `mapstructure:"failure-ratio"`
	OpenTimeout         time.Duration `mapstructure:"open-timeout"`
}

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

type RedisOptions struct {
	Addr                string `mapstructure:"addr"`
	Password            string `mapstructure:"password"`
	DB                  int    `mapstructure:"db"`
	PoolSize            int    `mapstructure:"pool-size"`
	MinIdleConns        int    `mapstructure:"min-idle-conns"`
	DialTimeoutSeconds  int    `mapstructure:"dial-timeout-seconds"`
	ReadTimeoutSeconds  int    `mapstructure:"read-timeout-seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write-timeout-seconds"`
	PingTimeoutSeconds  int    `mapstructure:"ping-timeout-seconds"`
}

func NewOptions() *Options {
	return &Options{
		Forecast: ForecastOptions{
			GRPCAddr:      "127.0.0.1:5007",
			HTTPAddr:      "127.0.0.1:8089",
			MetricsAddr:   "127.0.0.1:9109",
			ServiceName:   "vpp-forecast",
			CycleInterval: 15 * time.Minute,
			HorizonSteps:  4,
			StepSeconds:   900,
			HistoryWindow: 168 * time.Hour,
			Targets:       nil,
		},
		Telemetry: UpstreamOptions{
			GRPCAddr:       "127.0.0.1:5003",
			Timeout:        5 * time.Second,
			CircuitBreaker: defaultBreaker(),
		},
		Postgres: DatabaseOptions{
			Driver: "postgres",
			Host:   "127.0.0.1",
			Port:   5432,
			User:   "postgres",
			DBName: "forecast",
			Params: map[string]string{
				"sslmode":  "disable",
				"TimeZone": "Asia/Shanghai",
			},
			MaxOpenConns:           20,
			MaxIdleConns:           5,
			ConnMaxLifetimeSeconds: 1800,
			ConnMaxIdleTimeSeconds: 300,
		},
		Redis: RedisOptions{
			Addr:                "127.0.0.1:6379",
			DB:                  2,
			PoolSize:            10,
			MinIdleConns:        2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 3,
			PingTimeoutSeconds:  3,
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
	if o.Forecast.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("forecast.grpc-addr must not be empty"))
	}
	if o.Forecast.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("forecast.http-addr must not be empty"))
	}
	if o.Forecast.MetricsAddr == "" {
		errs = append(errs, fmt.Errorf("forecast.metrics-addr must not be empty"))
	}
	if o.Forecast.ServiceName == "" {
		errs = append(errs, fmt.Errorf("forecast.service-name must not be empty"))
	}
	if o.Forecast.CycleInterval <= 0 {
		errs = append(errs, fmt.Errorf("forecast.cycle-interval must be greater than 0"))
	}
	if o.Forecast.HorizonSteps < 1 {
		errs = append(errs, fmt.Errorf("forecast.horizon-steps must be at least 1"))
	}
	if o.Forecast.StepSeconds <= 0 {
		errs = append(errs, fmt.Errorf("forecast.step-seconds must be greater than 0"))
	}
	if o.Forecast.HistoryWindow <= 0 {
		errs = append(errs, fmt.Errorf("forecast.history-window must be greater than 0"))
	}
	if o.Telemetry.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("telemetry.grpc-addr must not be empty"))
	}
	if o.Redis.Addr == "" {
		errs = append(errs, fmt.Errorf("redis.addr must not be empty"))
	}
	if o.Postgres.DSN == "" {
		if o.Postgres.Host == "" {
			errs = append(errs, fmt.Errorf("postgres.host must not be empty (or set postgres.dsn)"))
		}
		if o.Postgres.User == "" {
			errs = append(errs, fmt.Errorf("postgres.user must not be empty (or set postgres.dsn)"))
		}
		if o.Postgres.DBName == "" {
			errs = append(errs, fmt.Errorf("postgres.dbname must not be empty (or set postgres.dsn)"))
		}
	}
	for i, r := range o.Forecast.Targets {
		if !r.Enabled {
			continue
		}
		prefix := fmt.Sprintf("forecast.targets[%d]", i)
		if r.TenantID == "" {
			errs = append(errs, fmt.Errorf("%s.tenant-id must not be empty", prefix))
		}
		if r.CUCode == "" {
			errs = append(errs, fmt.Errorf("%s.cu-code must not be empty", prefix))
		}
		if r.MetricName == "" {
			errs = append(errs, fmt.Errorf("%s.metric-name must not be empty", prefix))
		}
		switch r.Algorithm {
		case "moving_average":
			if r.MovingAverageWindow <= 0 {
				errs = append(errs, fmt.Errorf("%s.moving-average-window must be greater than 0", prefix))
			}
		case "same_period_prior":
			if r.SamePeriodLookbackDays <= 0 {
				errs = append(errs, fmt.Errorf("%s.same-period-lookback-days must be greater than 0", prefix))
			}
		default:
			errs = append(errs, fmt.Errorf("%s.algorithm must be moving_average or same_period_prior", prefix))
		}
	}
	return errs
}
