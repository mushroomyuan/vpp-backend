package options

import (
	"fmt"
	"time"
)

// Options holds all configurable parameters for the resource service.
// Fields are populated by viper.Unmarshal via mapstructure tags, mapping
// the YAML hierarchy (e.g. resource.grpc-addr) to nested structs.
type Options struct {
	Resource ResourceOptions `mapstructure:"resource"`
	Tracing  TracingOptions  `mapstructure:"tracing"`
	Database DatabaseOptions `mapstructure:"database"`
	Redis    RedisOptions    `mapstructure:"redis"`
	Kafka    KafkaOptions    `mapstructure:"kafka"`
}

type ResourceOptions struct {
	GRPCAddr     string        `mapstructure:"grpc-addr"`
	HTTPAddr     string        `mapstructure:"http-addr"`
	MetricsAddr  string        `mapstructure:"metrics-addr"`
	ServiceName  string        `mapstructure:"service-name"`
	ConsulAddr   string        `mapstructure:"consul-addr"`
	PollInterval time.Duration `mapstructure:"worker-poll-interval"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

// DatabaseOptions holds driver-agnostic database configuration.
//
// Common relational-database fields (Host, Port, User, Password, DBName) are
// explicit for readability and validation. Driver-specific connection
// parameters (e.g. sslmode/TimeZone for Postgres, charset/parseTime for MySQL)
// are placed under Params and interpreted by each concrete infra package.
//
// DSN is an optional escape hatch: if set in the config file, all structured
// fields are ignored and DSN is forwarded to the infrastructure layer as-is.
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

// KafkaOptions configures the resource event publisher.
type KafkaOptions struct {
	Brokers []string `mapstructure:"brokers"`
	Topic   string   `mapstructure:"topic"`
}

func NewOptions() *Options {
	return &Options{
		Resource: ResourceOptions{
			GRPCAddr:     ":9090",
			HTTPAddr:     ":8080",
			MetricsAddr:  ":9091",
			ServiceName:  "vpp-resource",
			PollInterval: 5 * time.Second,
		},
		Database: DatabaseOptions{
			Driver:                 "postgres",
			Host:                   "127.0.0.1",
			Port:                   5432,
			User:                   "postgres",
			DBName:                 "resource",
			Params:                 map[string]string{"sslmode": "disable", "TimeZone": "Asia/Shanghai"},
			MaxOpenConns:           50,
			MaxIdleConns:           10,
			ConnMaxLifetimeSeconds: 1800,
			ConnMaxIdleTimeSeconds: 300,
		},
		Redis: RedisOptions{
			Addr:                "127.0.0.1:6379",
			DB:                  0,
			PoolSize:            10,
			MinIdleConns:        2,
			DialTimeoutSeconds:  5,
			ReadTimeoutSeconds:  3,
			WriteTimeoutSeconds: 3,
			PingTimeoutSeconds:  3,
		},
		Kafka: KafkaOptions{
			Topic: "vpp.resource.events",
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Resource.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("resource.grpc-addr must not be empty"))
	}
	if o.Resource.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("resource.http-addr must not be empty"))
	}
	if o.Resource.ServiceName == "" {
		errs = append(errs, fmt.Errorf("resource.service-name must not be empty"))
	}
	if o.Database.Driver == "" {
		errs = append(errs, fmt.Errorf("database.driver must not be empty"))
	}
	if o.Redis.Addr == "" {
		errs = append(errs, fmt.Errorf("redis.addr must not be empty"))
	}
	// When DSN is explicitly set, structured fields are not required.
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
	return errs
}
