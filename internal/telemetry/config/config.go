package config

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/options"
)

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

	TrustProxyHeaders bool
	Authz             AuthzConfig
}

// AuthzConfig wires platform/authz for the telemetry service (C10c).
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
	a := opts.Telemetry.Auth
	az := a.Authz
	authzEnabled := a.TrustProxyHeaders && !az.Disabled

	return &Config{
		GRPCAddr:          opts.Telemetry.GRPCAddr,
		MetricsAddr:       opts.Telemetry.MetricsAddr,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		ServiceName:       opts.Telemetry.ServiceName,
		ConsulAddr:        opts.Telemetry.ConsulAddr,
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
