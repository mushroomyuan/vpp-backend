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
	}
}
