package config

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/options"
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
}

type KafkaConfig struct {
	Brokers       []string
	CommandTopic  string
	DispatchTopic string
	GroupID       string
}

func CreateFromOptions(opts *options.Options) *Config {
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
	}
}
