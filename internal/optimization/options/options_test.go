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
	if opts.Optimization.DecisionInterval <= 30*time.Second {
		t.Fatalf("default decision-interval %s is not above the 30s Telemetry cycle", opts.Optimization.DecisionInterval)
	}
	if !opts.Telemetry.CircuitBreaker.Enabled || !opts.Resource.CircuitBreaker.Enabled || !opts.Dispatch.CircuitBreaker.Enabled {
		t.Fatal("outbound circuit breakers must default to enabled")
	}
}

func TestValidate_RejectsEmptyAddrs(t *testing.T) {
	opts := NewOptions()
	opts.Optimization.HTTPAddr = ""
	opts.Telemetry.GRPCAddr = ""
	errs := opts.Validate()
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors, got %v", errs)
	}
}

func TestValidate_RejectsDecisionIntervalAtTelemetryCycle(t *testing.T) {
	opts := NewOptions()
	opts.Optimization.DecisionInterval = 30 * time.Second
	errs := opts.Validate()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
}

func TestValidate_EnabledSOCRuleRequiresBindings(t *testing.T) {
	opts := NewOptions()
	opts.Optimization.Rules.SOCThresholds = []SOCThresholdRuleOptions{{
		Enabled: true,
		MinSOC:  90,
		MaxSOC:  20,
	}}
	errs := opts.Validate()
	if len(errs) < 4 {
		t.Fatalf("expected cu-code/read/write/min<max errors, got %v", errs)
	}
}

func TestValidate_DisabledSOCRuleIsNotChecked(t *testing.T) {
	opts := NewOptions()
	opts.Optimization.Rules.SOCThresholds = []SOCThresholdRuleOptions{{
		Enabled: false,
		MinSOC:  90,
		MaxSOC:  20,
	}}
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("disabled rule must not be validated, got %v", errs)
	}
}
