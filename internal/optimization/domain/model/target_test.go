package model

import "testing"

func TestPointTarget_Accessors(t *testing.T) {
	pt := PointTarget{
		Tenant:   "tenant-1",
		Src:      SourceInternalRule,
		CUCode:   "cu-1",
		PointKey: "active_power_kw",
		Value:    FloatCommandValue(10.5),
	}

	if got := pt.TenantID(); got != "tenant-1" {
		t.Errorf("TenantID() = %q, want %q", got, "tenant-1")
	}
	if got := pt.Source(); got != SourceInternalRule {
		t.Errorf("Source() = %q, want %q", got, SourceInternalRule)
	}
}

func TestAggregateTarget_Accessors(t *testing.T) {
	at := AggregateTarget{
		Tenant:     "tenant-1",
		Src:        SourceExternalDR,
		Scope:      []string{"cu-1", "cu-2"},
		Metric:     "active_power_kw",
		DeltaValue: -500,
	}

	if got := at.TenantID(); got != "tenant-1" {
		t.Errorf("TenantID() = %q, want %q", got, "tenant-1")
	}
	if got := at.Source(); got != SourceExternalDR {
		t.Errorf("Source() = %q, want %q", got, SourceExternalDR)
	}
}

// TestTarget_InterfaceCompleteness pins the two implementations that exist
// today. If a new Target implementation is added without updating this
// test, that's a signal (not a guarantee — Go gives no compile-time
// exhaustiveness check, see the package doc comment) to also check
// Allocate's type switch handles it.
func TestTarget_InterfaceCompleteness(t *testing.T) {
	var targets = []Target{
		PointTarget{Tenant: "t", Src: SourceInternalRule},
		AggregateTarget{Tenant: "t", Src: SourceExternalDR},
	}
	for _, target := range targets {
		if target.TenantID() != "t" {
			t.Errorf("TenantID() = %q, want %q", target.TenantID(), "t")
		}
	}
}
