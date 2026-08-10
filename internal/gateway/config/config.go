package config

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/gateway/options"
)

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

	TrustProxyHeaders bool
	Authz             AuthzConfig
}

// KafkaConfig holds connection parameters for the resource event consumer
// and the command-completed event producer.
// Brokers empty → consumer/publisher do not start (no-op).
type KafkaConfig struct {
	Brokers      []string
	Topic        string // resource lifecycle consume
	GroupID      string
	CommandTopic string // command completed produce
}

// AuthzConfig wires platform/authz for the gateway service (C10b).
type AuthzConfig struct {
	Enabled         bool
	Sync            bool
	RegisterCatalog bool

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
	a := opts.Gateway.Auth
	az := a.Authz
	authzEnabled := a.TrustProxyHeaders && !az.Disabled

	return &Config{
		GRPCAddr:          opts.Gateway.GRPCAddr,
		HTTPAddr:          opts.Gateway.HTTPAddr,
		MetricsAddr:       opts.Gateway.MetricsAddr,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		ServiceName:       opts.Gateway.ServiceName,
		ConsulAddr:        opts.Gateway.ConsulAddr,
		Kafka: KafkaConfig{
			Brokers:      opts.Kafka.Brokers,
			Topic:        opts.Kafka.Topic,
			GroupID:      opts.Kafka.GroupID,
			CommandTopic: opts.Kafka.CommandTopic,
		},
		TrustProxyHeaders: a.TrustProxyHeaders,
		Authz: AuthzConfig{
			Enabled:              authzEnabled,
			Sync:                 authzEnabled,
			RegisterCatalog:      authzEnabled && !az.DisableRegisterCatalog,
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
