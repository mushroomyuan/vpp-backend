package options

import (
	"fmt"
	"time"
)

// Options holds all configurable parameters for the dispatch service.
type Options struct {
	Dispatch DispatchOptions `mapstructure:"dispatch"`
	Tracing  TracingOptions  `mapstructure:"tracing"`
	Database DatabaseOptions `mapstructure:"database"`
	Gateway  GatewayOptions  `mapstructure:"gateway"`
	Kafka    KafkaOptions    `mapstructure:"kafka"`
}

type DispatchOptions struct {
	GRPCAddr              string        `mapstructure:"grpc-addr"`
	MetricsAddr           string        `mapstructure:"metrics-addr"`
	ServiceName           string        `mapstructure:"service-name"`
	ConsulAddr            string        `mapstructure:"consul-addr"`
	TimeoutScanInterval   time.Duration `mapstructure:"timeout-scan-interval"`
	DefaultCommandTimeout time.Duration `mapstructure:"default-command-timeout"`
	DefaultMaxRetries     int           `mapstructure:"default-max-retries"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
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

type GatewayOptions struct {
	GRPCAddr string `mapstructure:"grpc-addr"`
}

// KafkaOptions configures command-result consumer and task-event publisher.
// Brokers empty → consumers/publishers degrade to no-op.
type KafkaOptions struct {
	Brokers       []string `mapstructure:"brokers"`
	CommandTopic  string   `mapstructure:"command-topic"`
	DispatchTopic string   `mapstructure:"dispatch-topic"`
	GroupID       string   `mapstructure:"group-id"`
}

func NewOptions() *Options {
	return &Options{
		Dispatch: DispatchOptions{
			GRPCAddr:              ":5006",
			MetricsAddr:           ":9105",
			ServiceName:           "vpp-dispatch",
			TimeoutScanInterval:   10 * time.Second,
			DefaultCommandTimeout: 30 * time.Second,
			DefaultMaxRetries:     3,
		},
		Database: DatabaseOptions{
			Driver: "postgres",
			Host:   "127.0.0.1",
			Port:   5432,
			User:   "postgres",
			DBName: "dispatch",
			Params: map[string]string{
				"sslmode":  "disable",
				"TimeZone": "Asia/Shanghai",
			},
			MaxOpenConns:           20,
			MaxIdleConns:           5,
			ConnMaxLifetimeSeconds: 1800,
			ConnMaxIdleTimeSeconds: 300,
		},
		Gateway: GatewayOptions{
			GRPCAddr: "127.0.0.1:5005",
		},
		Kafka: KafkaOptions{
			CommandTopic:  "vpp.command.events",
			DispatchTopic: "vpp.dispatch.events",
			GroupID:       "vpp-dispatch-command-events",
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Dispatch.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("dispatch.grpc-addr must not be empty"))
	}
	if o.Dispatch.ServiceName == "" {
		errs = append(errs, fmt.Errorf("dispatch.service-name must not be empty"))
	}
	if o.Gateway.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("gateway.grpc-addr must not be empty"))
	}
	if o.Database.DSN == "" && o.Database.Host == "" {
		errs = append(errs, fmt.Errorf("database.host or database.dsn must be set"))
	}
	return errs
}
