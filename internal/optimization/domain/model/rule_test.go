package model

import "testing"

func TestDefaultRules_EmptyByDesign(t *testing.T) {
	rules := DefaultRules()
	if len(rules.SOCThresholds) != 0 {
		t.Errorf("DefaultRules() should have no baked-in thresholds (deployment-specific), got %d", len(rules.SOCThresholds))
	}
}

func TestSOCThresholdRule_FieldsRoundTrip(t *testing.T) {
	r := SOCThresholdRule{
		Enabled:          true,
		CUCode:           "cu-battery-1",
		ReadPointKey:     "soc",
		WritePointKey:    "active_power_setpoint_kw",
		MinSOC:           20,
		MaxSOC:           90,
		ChargePowerKW:    50,
		DischargePowerKW: -50,
	}

	if r.CUCode != "cu-battery-1" {
		t.Errorf("CUCode = %q, want %q", r.CUCode, "cu-battery-1")
	}
	if r.Cooldown != 0 {
		t.Errorf("Cooldown = %v, want 0 (means: use decision loop default)", r.Cooldown)
	}
}
