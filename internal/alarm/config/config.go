package config

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/service"
	"github.com/mushroomyuan/vpp-backend/alarm/options"
)

// Config is the application-level configuration passed through the wiring layer.
type Config struct {
	HTTPAddr    string
	MetricsAddr string
	ServiceName string

	TelemetryEndpoint string
	TelemetryInsecure bool

	Kafka KafkaConfig
	Rules service.Rules

	TrustProxyHeaders bool
	Authz             AuthzConfig
}

type KafkaConfig struct {
	Brokers         []string
	DispatchTopic   string
	SOETopic        string
	DispatchGroupID string
	SOEGroupID      string
}

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
	a := opts.Alarm.Auth
	az := a.Authz
	authzEnabled := a.TrustProxyHeaders && !az.Disabled

	return &Config{
		HTTPAddr:          opts.Alarm.HTTPAddr,
		MetricsAddr:       opts.Alarm.MetricsAddr,
		ServiceName:       opts.Alarm.ServiceName,
		TelemetryEndpoint: opts.Tracing.Endpoint,
		TelemetryInsecure: opts.Tracing.Insecure,
		Kafka: KafkaConfig{
			Brokers:         opts.Kafka.Brokers,
			DispatchTopic:   opts.Kafka.DispatchTopic,
			SOETopic:        opts.Kafka.SOETopic,
			DispatchGroupID: opts.Kafka.DispatchGroupID,
			SOEGroupID:      opts.Kafka.SOEGroupID,
		},
		Rules:             rulesFromOptions(opts.Alarm.Rules),
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

func rulesFromOptions(o options.RulesOptions) service.Rules {
	r := service.DefaultRules()
	r.DispatchTaskFailed.Enabled = o.DispatchTaskFailed.Enabled
	if sev, err := model.ParseSeverity(o.DispatchTaskFailed.Severity); err == nil {
		r.DispatchTaskFailed.Severity = sev
	}
	r.SOEDiscreteChange.Enabled = o.SOEDiscreteChange.Enabled
	if sev, err := model.ParseSeverity(o.SOEDiscreteChange.Severity); err == nil {
		r.SOEDiscreteChange.Severity = sev
	}
	r.SOEDiscreteChange.MetricNames = o.SOEDiscreteChange.MetricNames
	return r
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
