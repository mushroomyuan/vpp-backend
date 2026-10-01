package model

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

func TestPointRejectsAccessIncompatibleWithMetric(t *testing.T) {
	t.Parallel()

	_, err := NewPoint(CreatePointParams{
		ID: "p1", AssetID: "a1", CUID: "c1",
		MetricID:        string(contracts.MetricEnergyStorageStateOfCharge),
		ExternalAddress: "soc", AccessMode: AccessModeWrite, Scale: 1,
	})
	if err == nil {
		t.Fatal("expected write access rejection for read-only SOC")
	}
}

func TestPointReplaceBindingBumpsRevisions(t *testing.T) {
	t.Parallel()

	minValue := 0.0
	point, err := NewPoint(CreatePointParams{
		ID: "p1", AssetID: "a1", CUID: "c1",
		MetricID:        string(contracts.MetricElectricalActivePowerSetpoint),
		ExternalAddress: "set_power", AccessMode: AccessModeWrite, Scale: 1,
		SafetyConstraint: &PointSafetyConstraint{MinValue: &minValue},
	})
	if err != nil {
		t.Fatal(err)
	}
	maxValue := 50.0
	if err := point.ReplaceBinding(
		string(contracts.MetricElectricalActivePowerSetpoint),
		"set_power_kw", AccessModeWrite, 1, 0, true,
		&PointSafetyConstraint{MinValue: &minValue, MaxValue: &maxValue},
		1,
	); err != nil {
		t.Fatal(err)
	}
	if point.Revision != 2 || point.SafetyConstraint.Version != 2 {
		t.Fatalf("point revision=%d safety version=%d", point.Revision, point.SafetyConstraint.Version)
	}
}
