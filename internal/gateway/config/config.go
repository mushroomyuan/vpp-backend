package config

import "github.com/mushroomyuan/vpp-backend/gateway/options"

// Config is the application-level configuration passed through the wiring layer.
// Database and telemetry gRPC connection details are infrastructure concerns
// assembled separately in the composition root (server.go).
type Config struct {
	GRPCAddr          string
	HTTPAddr          string
	MetricsAddr       string
	TelemetryEndpoint string
	TelemetryInsecure bool
	ServiceName       string
	ConsulAddr        string
	Kafka             KafkaConfig
}

// KafkaConfig holds connection parameters for the resource event consumer.
// Brokers empty → consumer does not start.
type KafkaConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

func CreateFromOptions(opts *options.Options) *Config {
	return &Config{
		GRPCAddr:          opts.Gateway.GRPCAddr,
		HTTPAddr:          opts.Gateway.HTTPAddr,
		MetricsAddr:       opts.Gateway.MetricsAddr,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		ServiceName:       opts.Gateway.ServiceName,
		ConsulAddr:        opts.Gateway.ConsulAddr,
		Kafka: KafkaConfig{
			Brokers: opts.Kafka.Brokers,
			Topic:   opts.Kafka.Topic,
			GroupID: opts.Kafka.GroupID,
		},
	}
}
