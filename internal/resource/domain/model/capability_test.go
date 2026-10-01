package model

import (
	"testing"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

func TestNewCUCapability(t *testing.T) {
	t.Parallel()

	capability, err := NewCUCapability(CreateCUCapabilityParams{
		ID: "cap-1", TenantID: "tenant-1", CUID: "cu-1",
		CapabilityID:  string(contracts.CapabilityEnergyStorage),
		SchemaVersion: 1,
		Spec: []byte(`{
			"usable_energy_kwh": 200,
			"max_charge_power_kw": 50,
			"max_discharge_power_kw": 50
		}`),
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if capability.Version != 1 || capability.CapabilityID != contracts.CapabilityEnergyStorage {
		t.Fatalf("capability = %+v", capability)
	}
}

func TestCUCapabilityRejectsUnknownSpecField(t *testing.T) {
	t.Parallel()

	_, err := NewCUCapability(CreateCUCapabilityParams{
		ID: "cap-1", TenantID: "tenant-1", CUID: "cu-1",
		CapabilityID:  string(contracts.CapabilityEnergyStorage),
		SchemaVersion: 1,
		Spec: []byte(`{
			"usable_energy_kwh": 200,
			"max_charge_power_kw": 50,
			"max_discharge_power_kw": 50,
			"vendor_mode": "fast"
		}`),
	})
	if err == nil {
		t.Fatal("expected strict spec validation error")
	}
}

func TestCUCapabilityReplaceUsesOptimisticVersion(t *testing.T) {
	t.Parallel()

	capability, err := NewCUCapability(CreateCUCapabilityParams{
		ID: "cap-1", TenantID: "tenant-1", CUID: "cu-1",
		CapabilityID:  string(contracts.CapabilityEnergyStorage),
		SchemaVersion: 1,
		Spec: []byte(`{
			"usable_energy_kwh": 200,
			"max_charge_power_kw": 50,
			"max_discharge_power_kw": 50
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := capability.Replace(1, capability.Spec, true, 2); err == nil {
		t.Fatal("expected version conflict")
	}
	if err := capability.Replace(1, capability.Spec, true, 1); err != nil {
		t.Fatal(err)
	}
	if capability.Version != 2 {
		t.Fatalf("version = %d", capability.Version)
	}
}
