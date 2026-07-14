package options

import (
	"fmt"
	"time"
)

// Options holds all configurable parameters for the simulator service.
type Options struct {
	Simulator SimulatorOptions `mapstructure:"simulator"`
	Tracing   TracingOptions   `mapstructure:"tracing"`
	Resource  ResourceOptions  `mapstructure:"resource"`
	Gateway   GatewayOptions   `mapstructure:"gateway"`
	Runtime   RuntimeOptions   `mapstructure:"runtime"`
}

type SimulatorOptions struct {
	HTTPAddr    string `mapstructure:"http-addr"`
	MetricsAddr string `mapstructure:"metrics-addr"`
	ServiceName string `mapstructure:"service-name"`
	ConsulAddr  string `mapstructure:"consul-addr"`
	TenantID    string `mapstructure:"tenant-id"`
}

type TracingOptions struct {
	Endpoint string `mapstructure:"endpoint"`
	Insecure bool   `mapstructure:"insecure"`
}

type ResourceOptions struct {
	GRPCAddr string `mapstructure:"grpc-addr"`
}

type GatewayOptions struct {
	HTTPAddr string `mapstructure:"http-addr"` // e.g. http://127.0.0.1:8083
}

type RuntimeOptions struct {
	TickInterval      time.Duration `mapstructure:"tick-interval"`
	PublishEnabled    bool          `mapstructure:"publish-enabled"` // false → Tick only, no Gateway ingest
	TraceSampleEvery  int           `mapstructure:"trace-sample-every"` // create tick/publish spans every N ticks; 1 = every tick
	SiteIDs           []string      `mapstructure:"site-ids"`
	CUIDs             []string      `mapstructure:"cu-ids"`
	RequireProvider   string        `mapstructure:"require-provider"` // default "simulator"; empty = no provider filter
}

func NewOptions() *Options {
	return &Options{
		Simulator: SimulatorOptions{
			HTTPAddr:    ":8084",
			MetricsAddr: ":9106",
			ServiceName: "vpp-simulator",
			TenantID:    "default",
		},
		Resource: ResourceOptions{
			GRPCAddr: "127.0.0.1:5002",
		},
		Gateway: GatewayOptions{
			HTTPAddr: "http://127.0.0.1:8083",
		},
		Runtime: RuntimeOptions{
			TickInterval:     30 * time.Second,
			PublishEnabled:   true,
			TraceSampleEvery: 6, // ~1 sampled tick/min at 10s interval; raise to cut Jaeger volume
			RequireProvider:  "simulator",
		},
	}
}

func (o *Options) Validate() []error {
	var errs []error
	if o.Simulator.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("simulator.http-addr must not be empty"))
	}
	if o.Simulator.ServiceName == "" {
		errs = append(errs, fmt.Errorf("simulator.service-name must not be empty"))
	}
	if o.Simulator.TenantID == "" {
		errs = append(errs, fmt.Errorf("simulator.tenant-id must not be empty"))
	}
	if o.Resource.GRPCAddr == "" {
		errs = append(errs, fmt.Errorf("resource.grpc-addr must not be empty"))
	}
	if o.Gateway.HTTPAddr == "" {
		errs = append(errs, fmt.Errorf("gateway.http-addr must not be empty"))
	}
	if o.Runtime.TickInterval <= 0 {
		errs = append(errs, fmt.Errorf("runtime.tick-interval must be > 0"))
	}
	if o.Runtime.TraceSampleEvery < 0 {
		errs = append(errs, fmt.Errorf("runtime.trace-sample-every must be >= 0"))
	}
	return errs
}
