package model

import "testing"

func TestForecastTarget_FieldsRoundTrip(t *testing.T) {
	tgt := ForecastTarget{
		Enabled:                true,
		TenantID:               "tenant-a",
		CUCode:                 "cu-battery-1",
		MetricName:             "active_power_kw",
		Algorithm:              AlgorithmMovingAverage,
		MovingAverageWindow:    8,
		SamePeriodLookbackDays: 7,
	}

	if tgt.TenantID != "tenant-a" || tgt.CUCode != "cu-battery-1" || tgt.MetricName != "active_power_kw" {
		t.Errorf("identity fields: %+v", tgt)
	}
	if tgt.Algorithm != AlgorithmMovingAverage {
		t.Errorf("Algorithm = %q, want %q", tgt.Algorithm, AlgorithmMovingAverage)
	}
	if !tgt.Enabled {
		t.Error("Enabled = false, want true")
	}
	if tgt.MovingAverageWindow != 8 || tgt.SamePeriodLookbackDays != 7 {
		t.Errorf("sidecar params window=%d lookback=%d", tgt.MovingAverageWindow, tgt.SamePeriodLookbackDays)
	}
}

func TestForecastTarget_IsPlainStructNotInterface(t *testing.T) {
	// Compile-time conversion: ForecastTarget is one named struct, not an
	// interface (design plan §5). Field sets must stay identical.
	type flat struct {
		Enabled                bool
		TenantID               string
		CUCode                 string
		MetricName             string
		Algorithm              AlgorithmID
		MovingAverageWindow    int
		SamePeriodLookbackDays int
	}
	_ = flat(ForecastTarget{})
}
