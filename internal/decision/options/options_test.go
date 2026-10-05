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
	if opts.Decision.DecisionInterval <= 30*time.Second {
		t.Fatalf("default decision-interval %s is not above the 30s Telemetry cycle", opts.Decision.DecisionInterval)
	}
	if opts.Decision.ServiceName != "vpp-decision" {
		t.Fatalf("ServiceName = %q", opts.Decision.ServiceName)
	}
	if opts.Database.DBName != "decision" {
		t.Fatalf("DBName = %q", opts.Database.DBName)
	}
	if !opts.Telemetry.CircuitBreaker.Enabled || !opts.Resource.CircuitBreaker.Enabled || !opts.Dispatch.CircuitBreaker.Enabled {
		t.Fatal("outbound circuit breakers must default to enabled")
	}
}

func TestValidate_RejectsEmptyAddrs(t *testing.T) {
	opts := NewOptions()
	opts.Decision.HTTPAddr = ""
	opts.Decision.GRPCAddr = ""
	opts.Telemetry.GRPCAddr = ""
	errs := opts.Validate()
	if len(errs) < 3 {
		t.Fatalf("expected at least 3 errors, got %v", errs)
	}
}

func TestValidate_RejectsDecisionIntervalAtTelemetryCycle(t *testing.T) {
	opts := NewOptions()
	opts.Decision.DecisionInterval = 30 * time.Second
	errs := opts.Validate()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
}

func TestValidate_RequiresDatabaseUnlessDSN(t *testing.T) {
	opts := NewOptions()
	opts.Database.Host = ""
	opts.Database.User = ""
	opts.Database.DBName = ""
	if errs := opts.Validate(); len(errs) != 3 {
		t.Fatalf("expected host/user/dbname errors, got %v", errs)
	}

	opts.Database.DSN = "postgres://localhost/decision"
	if errs := opts.Validate(); len(errs) != 0 {
		t.Fatalf("DSN should skip structured database fields, got %v", errs)
	}
}
