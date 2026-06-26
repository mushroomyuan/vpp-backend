package config

import "github.com/mushroomyuan/vpp-backend/telemetry/options"

// Config is the application-level configuration for the telemetry service.
// Infrastructure connection details (TimescaleDB, Redis, Kafka) are assembled
// in the composition root (server.go) and never leak into this type.
type Config struct {
	GRPCAddr          string
	MetricsAddr       string
	TelemetryEndpoint string
	TelemetryInsecure bool
	ServiceName       string
	ConsulAddr        string
}

func CreateFromOptions(opts *options.Options) *Config {
	return &Config{
		GRPCAddr:          opts.Telemetry.GRPCAddr,
		MetricsAddr:       opts.Telemetry.MetricsAddr,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		ServiceName:       opts.Telemetry.ServiceName,
		ConsulAddr:        opts.Telemetry.ConsulAddr,
	}
}
