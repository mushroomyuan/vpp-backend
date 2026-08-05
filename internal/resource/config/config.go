package config

import (
	"time"

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
	Authz             AuthzConfig
}

// KafkaConfig holds connection parameters for the resource event publisher.
// Brokers empty means Kafka is not configured; events will be dropped (no-op).
type KafkaConfig struct {
	Brokers []string
	Topic   string
}

// AuthzConfig wires platform/authz for the resource service (C7).
type AuthzConfig struct {
	// Enabled builds a local PermissionChecker when TrustProxyHeaders is true
	// (or when Sync is explicitly requested).
	Enabled bool
	// Sync starts the Casdoor policy syncer.
	Sync bool

	CasdoorURL      string
	CasdoorOrg      string
	CasdoorApp      string
	CasdoorUsername string
	CasdoorPassword string

	Owner                string
	ModelFilter          string
	SnapshotPath         string
	SyncInterval         time.Duration
	HealthyAfter         time.Duration
	StaleAfter           time.Duration
	AllowReadWhenInvalid bool
}

func CreateFromOptions(opts *options.Options) *Config {
	a := opts.Resource.Auth
	az := a.Authz

	authzEnabled := a.TrustProxyHeaders && !az.Disabled

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
		TrustProxyHeaders: a.TrustProxyHeaders,
		Authz: AuthzConfig{
			Enabled:              authzEnabled,
			Sync:                 authzEnabled, // B1 pull whenever PDP is on
			CasdoorURL:           defaultStr(az.CasdoorURL, "http://127.0.0.1:8000"),
			CasdoorOrg:           defaultStr(az.CasdoorOrg, "built-in"),
			CasdoorApp:           defaultStr(az.CasdoorApp, "app-built-in"),
			CasdoorUsername:      defaultStr(az.CasdoorUsername, "admin"),
			CasdoorPassword:      defaultStr(az.CasdoorPassword, "123"),
			Owner:                defaultStr(az.Owner, "default"),
			ModelFilter:          defaultStr(az.ModelFilter, "default/vpp-rbac"),
			SnapshotPath:         az.SnapshotPath,
			SyncInterval:         parseDuration(az.SyncInterval, 30*time.Second),
			HealthyAfter:         parseDuration(az.HealthyAfter, 5*time.Minute),
			StaleAfter:           parseDuration(az.StaleAfter, 30*time.Minute),
			AllowReadWhenInvalid: az.AllowReadWhenInvalid,
		},
	}
}

func defaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
