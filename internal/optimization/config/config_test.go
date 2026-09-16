package config

import (
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/mushroomyuan/vpp-backend/optimization/options"
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
	if cfg.HTTPAddr == "" || cfg.MetricsAddr == "" {
		t.Fatalf("HTTP/metrics addrs must be set, got http=%q metrics=%q", cfg.HTTPAddr, cfg.MetricsAddr)
	}
	if cfg.Telemetry.Addr == "" || cfg.Resource.Addr == "" || cfg.Dispatch.Addr == "" {
		t.Fatal("upstream gRPC addrs must be set")
	}
	if cfg.ServiceName != "vpp-optimization" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if len(cfg.TenantIDs) != 0 {
		t.Errorf("Default tenants should be empty, got %v", cfg.TenantIDs)
	}
	if len(cfg.Rules.SOCThresholds) != 0 {
		t.Errorf("Default rules should be empty, got %d", len(cfg.Rules.SOCThresholds))
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
	opts.Optimization.DecisionInterval = 90 * time.Second
	opts.Optimization.DefaultCooldown = 0
	cfg := CreateFromOptions(opts)
	if cfg.DefaultCooldown != 180*time.Second {
		t.Errorf("DefaultCooldown = %s, want 180s", cfg.DefaultCooldown)
	}
}

func TestCreateFromOptions_MapsSOCRules(t *testing.T) {
	opts := options.NewOptions()
	opts.Optimization.TenantIDs = []string{"tenant-1"}
	opts.Optimization.Rules.SOCThresholds = []options.SOCThresholdRuleOptions{{
		Enabled:          true,
		CUCode:           "cu-battery-1",
		ReadPointKey:     "soc",
		WritePointKey:    "active_power_setpoint_kw",
		MinSOC:           20,
		MaxSOC:           90,
		ChargePowerKW:    50,
		DischargePowerKW: -50,
		Cooldown:         3 * time.Minute,
	}}
	cfg := CreateFromOptions(opts)
	if len(cfg.TenantIDs) != 1 || cfg.TenantIDs[0] != "tenant-1" {
		t.Fatalf("TenantIDs = %v", cfg.TenantIDs)
	}
	if len(cfg.Rules.SOCThresholds) != 1 {
		t.Fatalf("expected 1 SOC rule, got %d", len(cfg.Rules.SOCThresholds))
	}
	r := cfg.Rules.SOCThresholds[0]
	if !r.Enabled || r.CUCode != "cu-battery-1" || r.ReadPointKey != "soc" || r.WritePointKey != "active_power_setpoint_kw" {
		t.Errorf("unexpected rule bindings: %+v", r)
	}
	if r.MinSOC != 20 || r.MaxSOC != 90 || r.ChargePowerKW != 50 || r.DischargePowerKW != -50 {
		t.Errorf("unexpected thresholds: %+v", r)
	}
	if r.Cooldown != 3*time.Minute {
		t.Errorf("Cooldown = %s, want 3m", r.Cooldown)
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
	if cfg.HTTPAddr != "127.0.0.1:8088" || cfg.MetricsAddr != "127.0.0.1:9108" {
		t.Errorf("listen addrs http=%q metrics=%q", cfg.HTTPAddr, cfg.MetricsAddr)
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
}

func repoYAML(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../../config/optimization.yaml",
		"../../config/optimization.yaml",
		"config/optimization.yaml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("could not find config/optimization.yaml relative to the test cwd")
	return ""
}
