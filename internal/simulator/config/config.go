package config

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/simulator/options"
)

// Config is the application-level configuration passed through the wiring layer.
type Config struct {
	HTTPAddr    string
	MetricsAddr string
	ServiceName string
	ConsulAddr  string
	TenantID    string

	TelemetryEndpoint string
	TelemetryInsecure bool

	ResourceGRPCAddr string
	GatewayHTTPAddr  string

	TickInterval     time.Duration
	PublishEnabled   bool
	TraceSampleEvery int
	SiteIDs          []string
	CUIDs            []string
	RequireProvider  string
}

func CreateFromOptions(opts *options.Options) *Config {
	sampleEvery := opts.Runtime.TraceSampleEvery
	if sampleEvery <= 0 {
		sampleEvery = 1 // 0/negative in YAML → treat as “every tick”
	}
	return &Config{
		HTTPAddr:          opts.Simulator.HTTPAddr,
		MetricsAddr:       opts.Simulator.MetricsAddr,
		ServiceName:       opts.Simulator.ServiceName,
		ConsulAddr:        opts.Simulator.ConsulAddr,
		TenantID:          opts.Simulator.TenantID,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		ResourceGRPCAddr:  opts.Resource.GRPCAddr,
		GatewayHTTPAddr:   opts.Gateway.HTTPAddr,
		TickInterval:      opts.Runtime.TickInterval,
		PublishEnabled:    opts.Runtime.PublishEnabled,
		TraceSampleEvery:  sampleEvery,
		SiteIDs:           append([]string(nil), opts.Runtime.SiteIDs...),
		CUIDs:             append([]string(nil), opts.Runtime.CUIDs...),
		RequireProvider:   opts.Runtime.RequireProvider,
	}
}
