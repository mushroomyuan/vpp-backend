package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

type EnergyStorageSpec struct {
	UsableEnergyKWh     float64 `json:"usable_energy_kwh"`
	MaxChargePowerKW    float64 `json:"max_charge_power_kw"`
	MaxDischargePowerKW float64 `json:"max_discharge_power_kw"`
}

type PowerGenerationSpec struct {
	RatedPowerKW       float64 `json:"rated_power_kw"`
	MinPowerKW         float64 `json:"min_power_kw"`
	MaxRampKWPerMinute float64 `json:"max_ramp_kw_per_minute"`
}

type PowerConsumptionSpec struct {
	RatedPowerKW float64 `json:"rated_power_kw"`
}

type LoadFlexibilitySpec struct {
	MaxReductionKW float64 `json:"max_reduction_kw"`
	MaxIncreaseKW  float64 `json:"max_increase_kw"`
}

type ReactivePowerControlSpec struct {
	MaxAbsKVar float64 `json:"max_abs_kvar"`
}

type EVChargingSpec struct {
	MaxChargePowerKW float64 `json:"max_charge_power_kw"`
}

type EVVehicleToGridSpec struct {
	MaxExportPowerKW float64 `json:"max_export_power_kw"`
}

func ValidateCapabilitySpec(capabilityID CapabilityID, schemaVersion int, raw []byte) error {
	descriptor, ok := LookupCapability(capabilityID)
	if !ok {
		return fmt.Errorf("contracts: unknown capability id %q", capabilityID)
	}
	if schemaVersion != descriptor.SchemaVersion {
		return fmt.Errorf(
			"contracts: unsupported schema version %d for %q; want %d",
			schemaVersion, capabilityID, descriptor.SchemaVersion,
		)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("contracts: capability spec is required")
	}

	switch capabilityID {
	case CapabilityEnergyStorage:
		var spec EnergyStorageSpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		if err := positive("usable_energy_kwh", spec.UsableEnergyKWh); err != nil {
			return err
		}
		if err := positive("max_charge_power_kw", spec.MaxChargePowerKW); err != nil {
			return err
		}
		return positive("max_discharge_power_kw", spec.MaxDischargePowerKW)
	case CapabilityPowerGeneration:
		var spec PowerGenerationSpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		if err := positive("rated_power_kw", spec.RatedPowerKW); err != nil {
			return err
		}
		if err := nonNegative("min_power_kw", spec.MinPowerKW); err != nil {
			return err
		}
		if spec.MinPowerKW > spec.RatedPowerKW {
			return fmt.Errorf("contracts: min_power_kw must not exceed rated_power_kw")
		}
		return nonNegative("max_ramp_kw_per_minute", spec.MaxRampKWPerMinute)
	case CapabilityPowerConsumption:
		var spec PowerConsumptionSpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		return positive("rated_power_kw", spec.RatedPowerKW)
	case CapabilityLoadFlexibility:
		var spec LoadFlexibilitySpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		if err := nonNegative("max_reduction_kw", spec.MaxReductionKW); err != nil {
			return err
		}
		if err := nonNegative("max_increase_kw", spec.MaxIncreaseKW); err != nil {
			return err
		}
		if spec.MaxReductionKW == 0 && spec.MaxIncreaseKW == 0 {
			return fmt.Errorf("contracts: at least one flexibility limit must be positive")
		}
		return nil
	case CapabilityReactivePowerControl:
		var spec ReactivePowerControlSpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		return positive("max_abs_kvar", spec.MaxAbsKVar)
	case CapabilityEVCharging:
		var spec EVChargingSpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		return positive("max_charge_power_kw", spec.MaxChargePowerKW)
	case CapabilityEVVehicleToGrid:
		var spec EVVehicleToGridSpec
		if err := decodeStrict(raw, &spec); err != nil {
			return err
		}
		return positive("max_export_power_kw", spec.MaxExportPowerKW)
	default:
		return fmt.Errorf("contracts: no validator for capability %q", capabilityID)
	}
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("contracts: invalid capability spec: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("contracts: capability spec contains multiple JSON values")
		}
		return fmt.Errorf("contracts: invalid capability spec: %w", err)
	}
	return nil
}

func positive(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return fmt.Errorf("contracts: %s must be finite and positive", name)
	}
	return nil
}

func nonNegative(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return fmt.Errorf("contracts: %s must be finite and non-negative", name)
	}
	return nil
}
