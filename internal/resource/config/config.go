package config

import (
	"github.com/mushroomyuan/vpp-backend/resource/application/worker"
	"github.com/mushroomyuan/vpp-backend/resource/options"
)

// Config is the application-level configuration passed through the wiring
// layer. It contains only concerns the application cares about:
// network addresses, telemetry, and worker tuning.
// Database connection details are an infrastructure concern and live in the
// infrastructure package itself (postgres.Config), assembled in the
// composition root (server.go).
type Config struct {
	GRPCAddr          string
	HTTPAddr          string
	MetricsAddr       string
	TelemetryEndpoint string
	TelemetryInsecure bool
	ServiceName       string
	ConsulAddr        string
	WorkerConfig      worker.ImportWorkerConfig
	Kafka             KafkaConfig
	// TrustProxyHeaders enables Resource HTTP auth (X-Userinfo + tenant + RBAC).
	TrustProxyHeaders bool
}

// KafkaConfig holds connection parameters for the resource event publisher.
// Brokers empty means Kafka is not configured; events will be dropped (no-op).
type KafkaConfig struct {
	Brokers []string
	Topic   string
}

func CreateFromOptions(opts *options.Options) *Config {
	return &Config{
		GRPCAddr:          opts.Resource.GRPCAddr,
		HTTPAddr:          opts.Resource.HTTPAddr,
		MetricsAddr:       opts.Resource.MetricsAddr,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		ServiceName:       opts.Resource.ServiceName,
		ConsulAddr:        opts.Resource.ConsulAddr,
		WorkerConfig: worker.ImportWorkerConfig{
			PollInterval: opts.Resource.PollInterval,
		},
		Kafka: KafkaConfig{
			Brokers: opts.Kafka.Brokers,
			Topic:   opts.Kafka.Topic,
		},
		TrustProxyHeaders: opts.Resource.Auth.TrustProxyHeaders,
	}
}
