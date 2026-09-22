package model

import "testing"

func TestPointTarget_Accessors(t *testing.T) {
	pt := PointTarget{
		Tenant:   "tenant-1",
		Src:      SourceInternalRule,
		Rule:     RuleSOCThreshold,
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
	if got := pt.RuleID(); got != string(RuleSOCThreshold) {
		t.Errorf("RuleID() = %q, want %q", got, RuleSOCThreshold)
	}
}

// TestPointTarget_RuleIDEmptyWhenUnset guards the zero-value case: a
// PointTarget built without a Rule (e.g. in older call sites that predate
// the 2026-09 review fix) reports an empty RuleID rather than panicking or
// guessing — callers (ObserveRulesFired) already treat "" as "unknown".
func TestPointTarget_RuleIDEmptyWhenUnset(t *testing.T) {
	pt := PointTarget{Tenant: "tenant-1", Src: SourceInternalRule}
	if got := pt.RuleID(); got != "" {
		t.Errorf("RuleID() = %q, want empty for a PointTarget with no Rule set", got)
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
	if got := at.RuleID(); got != "" {
		t.Errorf("RuleID() = %q, want empty (AggregateTarget has no single rule to attribute to)", got)
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
