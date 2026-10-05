package config

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/mushroomyuan/vpp-backend/decision/options"
)

func TestDefault_DecisionIntervalExceedsTelemetryCycle(t *testing.T) {
	cfg := Default()
	const telemetryCycle = 30 * time.Second
	if cfg.DecisionInterval <= telemetryCycle {
		t.Errorf("DecisionInterval = %s, want > %s (Telemetry/Simulator collection cycle)", cfg.DecisionInterval, telemetryCycle)
	}
	if cfg.DefaultCooldown < 2*cfg.DecisionInterval {
		t.Errorf("DefaultCooldown = %s, want >= 2× interval (%s)", cfg.DefaultCooldown, 2*cfg.DecisionInterval)
	}
}

func TestDefault_ListenAddrs(t *testing.T) {
	cfg := Default()
	if cfg.HTTPAddr == "" || cfg.GRPCAddr == "" || cfg.MetricsAddr == "" {
		t.Fatalf("listen addrs must be set, got http=%q grpc=%q metrics=%q", cfg.HTTPAddr, cfg.GRPCAddr, cfg.MetricsAddr)
	}
	if cfg.Telemetry.Addr == "" || cfg.Resource.Addr == "" || cfg.Dispatch.Addr == "" {
		t.Fatal("upstream gRPC addrs must be set")
	}
	if cfg.ServiceName != "vpp-decision" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.Database.Driver != "postgres" || cfg.Database.DBName != "decision" {
		t.Errorf("database driver=%q dbname=%q", cfg.Database.Driver, cfg.Database.DBName)
	}
}

func TestDefault_BreakersEnabled(t *testing.T) {
	cfg := Default()
	if cfg.Telemetry.Breaker == nil || cfg.Resource.Breaker == nil || cfg.Dispatch.Breaker == nil {
		t.Fatal("CreateFromOptions(NewOptions()) must construct all three outbound breakers")
	}
}

func TestCreateFromOptions_ZeroCooldownBecomesTwiceInterval(t *testing.T) {
	opts := options.NewOptions()
	opts.Decision.DecisionInterval = 90 * time.Second
	opts.Decision.DefaultCooldown = 0
	cfg := CreateFromOptions(opts)
	if cfg.DefaultCooldown != 180*time.Second {
		t.Errorf("DefaultCooldown = %s, want 180s", cfg.DefaultCooldown)
	}
}

func TestCreateFromOptions_DisabledBreakerIsNil(t *testing.T) {
	opts := options.NewOptions()
	opts.Dispatch.CircuitBreaker.Enabled = false
	cfg := CreateFromOptions(opts)
	if cfg.Dispatch.Breaker != nil {
		t.Fatal("disabled dispatch breaker must be nil")
	}
	if cfg.Telemetry.Breaker == nil {
		t.Fatal("telemetry breaker should still be on")
	}
}

func TestCreateFromOptions_CopiesDatabaseParams(t *testing.T) {
	opts := options.NewOptions()
	opts.Database.Params["sslmode"] = "require"
	cfg := CreateFromOptions(opts)
	opts.Database.Params["sslmode"] = "disable"
	if cfg.Database.Params["sslmode"] != "require" {
		t.Fatalf("database params were not copied, got %v", cfg.Database.Params)
	}
}

func TestRepoYAML_UnmarshalsAndValidates(t *testing.T) {
	path := repoYAML(t)
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	opts := options.NewOptions()
	if err := v.Unmarshal(opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("repo YAML should validate: %v", errs)
	}
	cfg := CreateFromOptions(opts)
	if cfg.DecisionInterval != 60*time.Second {
		t.Errorf("DecisionInterval = %s, want 60s", cfg.DecisionInterval)
	}
	if cfg.DefaultCooldown != 2*time.Minute {
		t.Errorf("DefaultCooldown = %s, want 2m", cfg.DefaultCooldown)
	}
	if cfg.ScopeCacheTTL != 30*time.Second || cfg.ScopeCacheMaxAge != 5*time.Minute || cfg.TelemetryStaleAge != 90*time.Second {
		t.Errorf("cache ttl=%s max=%s stale=%s", cfg.ScopeCacheTTL, cfg.ScopeCacheMaxAge, cfg.TelemetryStaleAge)
	}
	if cfg.HTTPAddr != "127.0.0.1:8088" || cfg.GRPCAddr != ":5008" || cfg.MetricsAddr != ":9108" {
		t.Errorf("listen addrs http=%q grpc=%q metrics=%q", cfg.HTTPAddr, cfg.GRPCAddr, cfg.MetricsAddr)
	}
	if cfg.Database.DBName != "decision" || cfg.Database.Host != "127.0.0.1" || cfg.Database.Port != 5432 {
		t.Errorf("database host=%q port=%d dbname=%q", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)
	}
	if cfg.Telemetry.Addr != "127.0.0.1:5003" || cfg.Resource.Addr != "127.0.0.1:5002" || cfg.Dispatch.Addr != "127.0.0.1:5006" {
		t.Errorf("upstream addrs tel=%q res=%q dis=%q", cfg.Telemetry.Addr, cfg.Resource.Addr, cfg.Dispatch.Addr)
	}
	if cfg.Telemetry.Breaker == nil || cfg.Resource.Breaker == nil || cfg.Dispatch.Breaker == nil {
		t.Fatal("repo YAML defaults all three circuit-breakers to enabled")
	}
	if cfg.TelemetryEndpoint != "127.0.0.1:4318" || !cfg.TelemetryInsecure {
		t.Errorf("tracing endpoint=%q insecure=%v", cfg.TelemetryEndpoint, cfg.TelemetryInsecure)
	}
	if cfg.TrustProxyHeaders {
		t.Fatal("repo YAML must leave auth.trust-proxy-headers false")
	}
	if !cfg.Authz.DenyWritesWhenStale || cfg.Authz.Enabled {
		t.Fatalf("authz enabled=%v denyWrites=%v", cfg.Authz.Enabled, cfg.Authz.DenyWritesWhenStale)
	}
}

func repoYAML(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../../config/decision.yaml",
		"../../config/decision.yaml",
		"config/decision.yaml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("could not find config/decision.yaml relative to the test cwd")
	return ""
}
