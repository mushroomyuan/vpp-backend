package context

import (
	"encoding/json"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

// EnergyStorageSpec returns the enabled energy.storage.v1 spec on a resolved CU.
// ok is false when the capability is missing, disabled, or fails the contract validator.
func EnergyStorageSpec(cu port.ResolvedCU) (contracts.EnergyStorageSpec, bool) {
	for _, cap := range cu.Capabilities {
		if cap.CapabilityID != contracts.CapabilityEnergyStorage || !cap.Enabled {
			continue
		}
		raw, err := json.Marshal(cap.Spec)
		if err != nil {
			return contracts.EnergyStorageSpec{}, false
		}
		if err := contracts.ValidateCapabilitySpec(cap.CapabilityID, int(cap.SchemaVersion), raw); err != nil {
			return contracts.EnergyStorageSpec{}, false
		}
		var spec contracts.EnergyStorageSpec
		if err := json.Unmarshal(raw, &spec); err != nil {
			return contracts.EnergyStorageSpec{}, false
		}
		return spec, true
	}
	return contracts.EnergyStorageSpec{}, false
}

// SetpointBinding returns the enabled write binding for the active-power setpoint.
func SetpointBinding(cu port.ResolvedCU) (port.ResolvedBinding, bool) {
	for _, binding := range cu.Bindings {
		if binding.MetricID != contracts.MetricElectricalActivePowerSetpoint || !binding.Enabled {
			continue
		}
		if binding.Revision <= 0 {
			continue
		}
		switch binding.AccessMode {
		case port.BindingAccessWrite, port.BindingAccessReadWrite:
			return binding, true
		}
	}
	return port.ResolvedBinding{}, false
}
