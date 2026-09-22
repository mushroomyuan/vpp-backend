package config

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/mushroomyuan/vpp-backend/forecast/domain/model"
	"github.com/mushroomyuan/vpp-backend/forecast/options"
)

func TestDefault_ListenAddrs(t *testing.T) {
	cfg := Default()
	if cfg.GRPCAddr == "" || cfg.HTTPAddr == "" || cfg.MetricsAddr == "" {
		t.Fatalf("listen addrs must be set, got grpc=%q http=%q metrics=%q", cfg.GRPCAddr, cfg.HTTPAddr, cfg.MetricsAddr)
	}
	if cfg.ServiceName != "vpp-forecast" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.Telemetry.Addr == "" {
		t.Fatal("telemetry gRPC addr must be set")
	}
	if len(cfg.Targets) != 0 {
		t.Errorf("default targets should be empty, got %d", len(cfg.Targets))
	}
}

func TestDefault_RedisTTLCoversHorizonPlusOneCycle(t *testing.T) {
	cfg := Default()
	want := 4*900*time.Second + 15*time.Minute
	if cfg.RedisTTL != want {
		t.Errorf("RedisTTL = %s, want %s", cfg.RedisTTL, want)
	}
}

func TestDefault_BreakerEnabled(t *testing.T) {
	cfg := Default()
	if cfg.Telemetry.Breaker == nil {
		t.Fatal("CreateFromOptions(NewOptions()) must construct the telemetry breaker")
	}
}

func TestCreateFromOptions_DisabledBreakerIsNil(t *testing.T) {
	opts := options.NewOptions()
	opts.Telemetry.CircuitBreaker.Enabled = false
	cfg := CreateFromOptions(opts)
	if cfg.Telemetry.Breaker != nil {
		t.Fatal("disabled telemetry breaker must be nil")
	}
}

func TestCreateFromOptions_CopiesTargets(t *testing.T) {
	opts := options.NewOptions()
	opts.Forecast.Targets = []options.TargetOptions{{
		Enabled:             true,
		TenantID:            "tenant-a",
		CUCode:              "cu-battery-1",
		MetricName:          "active_power_kw",
		Algorithm:           "moving_average",
		MovingAverageWindow: 8,
	}}
	cfg := CreateFromOptions(opts)
	if len(cfg.Targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(cfg.Targets))
	}
	got := cfg.Targets[0]
	if !got.Enabled || got.TenantID != "tenant-a" || got.CUCode != "cu-battery-1" {
		t.Errorf("unexpected target: %+v", got)
	}
	if got.MetricName != "active_power_kw" || got.Algorithm != "moving_average" || got.MovingAverageWindow != 8 {
		t.Errorf("unexpected target bindings: %+v", got)
	}
	opts.Forecast.Targets[0].TenantID = "mutated"
	if cfg.Targets[0].TenantID != "tenant-a" {
		t.Fatal("CreateFromOptions must copy targets, not alias the options slice")
	}
}

func TestCreateFromOptions_PostgresAndRedis(t *testing.T) {
	cfg := Default()
	if cfg.Postgres.DBName != "forecast" || cfg.Postgres.Host != "127.0.0.1" {
		t.Errorf("postgres = %+v", cfg.Postgres)
	}
	if cfg.Redis.DB != 2 || cfg.Redis.Addr == "" {
		t.Errorf("redis = %+v", cfg.Redis)
	}
}

func TestCreateFromOptions_ZeroIntervalFallsBack(t *testing.T) {
	opts := options.NewOptions()
	opts.Forecast.CycleInterval = 0
	opts.Forecast.HorizonSteps = 0
	opts.Forecast.StepSeconds = 0
	opts.Forecast.HistoryWindow = 0
	cfg := CreateFromOptions(opts)
	if cfg.CycleInterval != 15*time.Minute {
		t.Errorf("CycleInterval = %s, want 15m", cfg.CycleInterval)
	}
	if cfg.HorizonSteps != 4 || cfg.StepSeconds != 900 {
		t.Errorf("horizon=%d step=%d", cfg.HorizonSteps, cfg.StepSeconds)
	}
	if cfg.HistoryWindow != 168*time.Hour {
		t.Errorf("HistoryWindow = %s, want 168h", cfg.HistoryWindow)
	}
}

func TestCreateFromOptions_RedisTTLFollowsHorizon(t *testing.T) {
	opts := options.NewOptions()
	opts.Forecast.HorizonSteps = 8
	opts.Forecast.StepSeconds = 900
	opts.Forecast.CycleInterval = 15 * time.Minute
	cfg := CreateFromOptions(opts)
	want := 8*900*time.Second + 15*time.Minute
	if cfg.RedisTTL != want {
		t.Errorf("RedisTTL = %s, want %s (horizon-steps×step-seconds + cycle-interval)", cfg.RedisTTL, want)
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
	if cfg.CycleInterval != 15*time.Minute {
		t.Errorf("CycleInterval = %s, want 15m", cfg.CycleInterval)
	}
	if cfg.HorizonSteps != 4 || cfg.StepSeconds != 900 {
		t.Errorf("horizon=%d step=%d, want 4 / 900", cfg.HorizonSteps, cfg.StepSeconds)
	}
	if cfg.HistoryWindow != 168*time.Hour {
		t.Errorf("HistoryWindow = %s, want 168h", cfg.HistoryWindow)
	}
	if cfg.GRPCAddr != "127.0.0.1:5007" || cfg.HTTPAddr != "127.0.0.1:8089" || cfg.MetricsAddr != "127.0.0.1:9109" {
		t.Errorf("listen addrs grpc=%q http=%q metrics=%q", cfg.GRPCAddr, cfg.HTTPAddr, cfg.MetricsAddr)
	}
	if cfg.ServiceName != "vpp-forecast" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.Telemetry.Addr != "127.0.0.1:5003" || cfg.Telemetry.Timeout != 5*time.Second {
		t.Errorf("telemetry addr=%q timeout=%s", cfg.Telemetry.Addr, cfg.Telemetry.Timeout)
	}
	if cfg.Telemetry.Breaker == nil {
		t.Fatal("repo YAML defaults the telemetry circuit-breaker to enabled")
	}
	if cfg.TelemetryEndpoint != "127.0.0.1:4318" || !cfg.TelemetryInsecure {
		t.Errorf("tracing endpoint=%q insecure=%v", cfg.TelemetryEndpoint, cfg.TelemetryInsecure)
	}
	if cfg.Postgres.DBName != "forecast" || cfg.Postgres.Host != "127.0.0.1" || cfg.Postgres.Password != "postgres123" {
		t.Errorf("postgres = %+v", cfg.Postgres)
	}
	if cfg.Postgres.Params["sslmode"] != "disable" || cfg.Postgres.Params["TimeZone"] != "Asia/Shanghai" {
		t.Errorf("postgres params = %v", cfg.Postgres.Params)
	}
	if cfg.Redis.DB != 2 || cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Errorf("redis = %+v", cfg.Redis)
	}
	if cfg.RedisTTL != 4*900*time.Second+15*time.Minute {
		t.Errorf("RedisTTL = %s, want horizon+cycle", cfg.RedisTTL)
	}
	if len(cfg.Targets) != 0 {
		t.Errorf("repo YAML targets should be empty, got %d", len(cfg.Targets))
	}
}

func TestUnmarshal_YAMLTargets(t *testing.T) {
	const snippet = `
forecast:
  targets:
    - enabled: true
      tenant-id: tenant-a
      cu-code: cu-battery-1
      metric-name: active_power_kw
      algorithm: moving_average
      moving-average-window: 8
    - enabled: true
      tenant-id: tenant-a
      cu-code: cu-pv-1
      metric-name: active_power_kw
      algorithm: same_period_prior
      same-period-lookback-days: 7
`
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(snippet)); err != nil {
		t.Fatalf("read snippet: %v", err)
	}
	opts := options.NewOptions()
	if err := v.Unmarshal(opts); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("snippet should validate: %v", errs)
	}
	cfg := CreateFromOptions(opts)
	if len(cfg.Targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(cfg.Targets))
	}
	ma := cfg.Targets[0]
	if ma.Algorithm != model.AlgorithmMovingAverage || ma.MovingAverageWindow != 8 || ma.CUCode != "cu-battery-1" {
		t.Errorf("moving_average target = %+v", ma)
	}
	sp := cfg.Targets[1]
	if sp.Algorithm != model.AlgorithmSamePeriodPrior || sp.SamePeriodLookbackDays != 7 || sp.CUCode != "cu-pv-1" {
		t.Errorf("same_period_prior target = %+v", sp)
	}
}

func repoYAML(t *testing.T) string {
	t.Helper()
	candidates := []string{
		"../../../config/forecast.yaml",
		"../../config/forecast.yaml",
		"config/forecast.yaml",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatal("could not find config/forecast.yaml relative to the test cwd")
	return ""
}
