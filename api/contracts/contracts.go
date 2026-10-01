// Package contracts defines the versioned metric and capability identities
// shared by VPP services. Database rows may reference these identities, but
// they cannot define or rename them.
package contracts

import (
	"fmt"
	"regexp"
)

type MetricID string
type CapabilityID string

const (
	MetricElectricalActivePower         MetricID = "electrical.active_power.v1"
	MetricElectricalReactivePower       MetricID = "electrical.reactive_power.v1"
	MetricElectricalActivePowerSetpoint MetricID = "electrical.active_power_setpoint.v1"
	MetricEnergyStorageStateOfCharge    MetricID = "energy_storage.state_of_charge.v1"
)

const (
	CapabilityEnergyStorage        CapabilityID = "energy.storage.v1"
	CapabilityPowerGeneration      CapabilityID = "power.generation.v1"
	CapabilityPowerConsumption     CapabilityID = "power.consumption.v1"
	CapabilityLoadFlexibility      CapabilityID = "load.flexibility.v1"
	CapabilityReactivePowerControl CapabilityID = "reactive_power.control.v1"
	CapabilityEVCharging           CapabilityID = "ev.charging.v1"
	CapabilityEVVehicleToGrid      CapabilityID = "ev.vehicle_to_grid.v1"
)

type ValueKind string

const (
	ValueKindFloat64 ValueKind = "float64"
	ValueKindInt64   ValueKind = "int64"
	ValueKindBool    ValueKind = "bool"
	ValueKindEnum    ValueKind = "enum"
)

type AccessMode string

const (
	AccessModeRead      AccessMode = "read"
	AccessModeWrite     AccessMode = "write"
	AccessModeReadWrite AccessMode = "read_write"
)

func (m AccessMode) IsValid() bool {
	switch m {
	case AccessModeRead, AccessModeWrite, AccessModeReadWrite:
		return true
	default:
		return false
	}
}

func (m AccessMode) AllowsRead() bool {
	return m == AccessModeRead || m == AccessModeReadWrite
}

func (m AccessMode) AllowsWrite() bool {
	return m == AccessModeWrite || m == AccessModeReadWrite
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+\.v[1-9][0-9]*$`)

func ValidateIDFormat(raw string) error {
	if !idPattern.MatchString(raw) {
		return fmt.Errorf("contracts: invalid versioned id %q", raw)
	}
	return nil
}

func ParseMetricID(raw string) (MetricID, error) {
	if err := ValidateIDFormat(raw); err != nil {
		return "", err
	}
	id := MetricID(raw)
	if _, ok := LookupMetric(id); !ok {
		return "", fmt.Errorf("contracts: unknown metric id %q", raw)
	}
	return id, nil
}

func ParseCapabilityID(raw string) (CapabilityID, error) {
	if err := ValidateIDFormat(raw); err != nil {
		return "", err
	}
	id := CapabilityID(raw)
	if _, ok := LookupCapability(id); !ok {
		return "", fmt.Errorf("contracts: unknown capability id %q", raw)
	}
	return id, nil
}
