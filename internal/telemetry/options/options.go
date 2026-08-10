package options

import "fmt"

// Options holds all configurable parameters for the telemetry service.
// Fields are populated by viper.Unmarshal via mapstructure tags, mapping
// the YAML hierarchy (e.g. telemetry.grpc-addr) to nested structs.
type Options struct {
	Telemetry   TelemetryOptions   `mapstructure:"telemetry"`
	Tracing     TracingOptions     `mapstructure:"tracing"`
	TimescaleDB TimescaleDBOptions `mapstructure:"timescaledb"`
	Redis       RedisOptions       `mapstructure:"redis"`
	Kafka       KafkaOptions       `mapstructure:"kafka"`
}

type TelemetryOptions struct {
	GRPCAddr    string `mapstructure:"grpc-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`
	ConsulAddr  string `mapstructure:"consul-addr"`

	// Auth configures gRPC identity middleware (x-userinfo / Casdoor) for read RPCs.
	Auth AuthOptions `mapstructure:"auth"`
}

// AuthOptions configures gRPC identity middleware for telemetry read APIs (C10c).
type AuthOptions struct {
	// TrustProxyHeaders requires valid x-userinfo when true.
	// When false, auth is bypassed for local direct :5003 debugging.
	TrustProxyHeaders bool `mapstructure:"trust-proxy-headers"`
	// Authz configures local Casbin PDP + Casdoor policy sync.
	Authz AuthzOptions `mapstructure:"authz"`
}

// AuthzOptions is the telemetry-service wiring for platform/authz.
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

	DisableRegisterCatalog bool `mapstructure:"disable-register-catalog"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

// TimescaleDBOptions mirrors timescaledb.Config. Using structured fields
// (not a raw DSN) is the default; DSN overrides all structured fields when set.
type TimescaleDBOptions struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`

	// DSN overrides all structured fields above when non-empty.
	DSN string `mapstructure:"dsn"`

	MaxConns int32 `mapstructure:"max-conns"`
	MinConns int32 `mapstructure:"min-conns"`
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

// KafkaOptions configures the SOE event publisher.
type KafkaOptions struct {
	Brokers []string `mapstructure:"brokers"`
	Topic   string   `mapstructure:"topic"`
}

func NewOptions() *Options {
	return &Options{
		Telemetry: TelemetryOptions{
			GRPCAddr:    ":9092",
			MetricsAddr: ":9093",
			ServiceName: "vpp-telemetry",
		},
		TimescaleDB: TimescaleDBOptions{
			Host:     "127.0.0.1",
			Port:     5432,
			User:     "postgres",
			DBName:   "telemetry",
			SSLMode:  "disable",
			MaxConns: 20,
			MinConns: 2,
		},
		Redis: RedisOptions{
			Addr:                "127.0.0.1:6379",
			DB:                  1,
			PoolSize:            10,
			MinIdleConns:        2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 3,
			PingTimeoutSeconds:  3,
		},
		Kafka: KafkaOptions{
			Topic: "vpp.soe.events",
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Telemetry.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("telemetry.grpc-addr must not be empty"))
	}
	if o.Telemetry.ServiceName == "" {
		errs = append(errs, fmt.Errorf("telemetry.service-name must not be empty"))
	}
	if o.Redis.Addr == "" {
		errs = append(errs, fmt.Errorf("redis.addr must not be empty"))
	}
	if o.TimescaleDB.DSN == "" {
		if o.TimescaleDB.Host == "" {
			errs = append(errs, fmt.Errorf("timescaledb.host must not be empty (or set timescaledb.dsn)"))
		}
		if o.TimescaleDB.User == "" {
			errs = append(errs, fmt.Errorf("timescaledb.user must not be empty (or set timescaledb.dsn)"))
		}
		if o.TimescaleDB.DBName == "" {
			errs = append(errs, fmt.Errorf("timescaledb.dbname must not be empty (or set timescaledb.dsn)"))
		}
	}
	return errs
}
