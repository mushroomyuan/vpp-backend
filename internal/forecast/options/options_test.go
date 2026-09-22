package options

import (
	"testing"
	"time"
)

func TestNewOptions_ValidatePasses(t *testing.T) {
	opts := NewOptions()
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("NewOptions() should be valid, got %v", errs)
	}
	if opts.Forecast.CycleInterval != 15*time.Minute {
		t.Errorf("default cycle-interval = %s, want 15m", opts.Forecast.CycleInterval)
	}
	if opts.Forecast.HorizonSteps != 4 || opts.Forecast.StepSeconds != 900 {
		t.Errorf("horizon-steps=%d step-seconds=%d", opts.Forecast.HorizonSteps, opts.Forecast.StepSeconds)
	}
	if !opts.Telemetry.CircuitBreaker.Enabled {
		t.Fatal("telemetry circuit breaker must default to enabled")
	}
	if opts.Redis.DB != 2 {
		t.Errorf("redis db = %d, want 2", opts.Redis.DB)
	}
	if opts.Postgres.DBName != "forecast" {
		t.Errorf("postgres dbname = %q, want forecast", opts.Postgres.DBName)
	}
}

func TestValidate_RejectsEmptyAddrs(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.GRPCAddr = ""
	opts.Telemetry.GRPCAddr = ""
	errs := opts.Validate()
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors, got %v", errs)
	}
}

func TestValidate_AllowsCycleIntervalBelowTelemetryCollection(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.CycleInterval = 15 * time.Second
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("cycle-interval is not a correctness floor, got %v", errs)
	}
}

func TestValidate_RejectsZeroCycleInterval(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.CycleInterval = 0
	errs := opts.Validate()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
}

func TestValidate_EnabledTargetRequiresBindings(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.Targets = []TargetOptions{{
		Enabled:   true,
		Algorithm: "moving_average",
	}}
	errs := opts.Validate()
	if len(errs) < 4 {
		t.Fatalf("expected tenant/cu/metric/window errors, got %v", errs)
	}
}

func TestValidate_DisabledTargetIsNotChecked(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.Targets = []TargetOptions{{
		Enabled: false,
	}}
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("disabled target must not be validated, got %v", errs)
	}
}

func TestValidate_RejectsUnknownAlgorithm(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.Targets = []TargetOptions{{
		Enabled:    true,
		TenantID:   "t",
		CUCode:     "cu",
		MetricName: "kw",
		Algorithm:  "neural_net",
	}}
	errs := opts.Validate()
	if len(errs) != 1 {
		t.Fatalf("expected 1 algorithm error, got %v", errs)
	}
}

func TestValidate_EnabledSamePeriodPriorRequiresLookback(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.Targets = []TargetOptions{{
		Enabled:    true,
		TenantID:   "t",
		CUCode:     "cu",
		MetricName: "kw",
		Algorithm:  "same_period_prior",
	}}
	errs := opts.Validate()
	if len(errs) != 1 {
		t.Fatalf("expected lookback-days error, got %v", errs)
	}
}

func TestValidate_EnabledMovingAverageIsValid(t *testing.T) {
	opts := NewOptions()
	opts.Forecast.Targets = []TargetOptions{{
		Enabled:             true,
		TenantID:            "tenant-a",
		CUCode:              "cu-battery-1",
		MetricName:          "active_power_kw",
		Algorithm:           "moving_average",
		MovingAverageWindow: 8,
	}}
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("valid moving_average target, got %v", errs)
	}
}
