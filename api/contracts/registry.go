package contracts

import "sort"

type MetricDescriptor struct {
	ID            MetricID
	DisplayName   string
	CanonicalUnit string
	ValueKind     ValueKind
	Readable      bool
	Writable      bool
	MinValue      *float64
	MaxValue      *float64
}

type CapabilityDescriptor struct {
	ID            CapabilityID
	DisplayName   string
	SchemaVersion int
}

func float64Ptr(v float64) *float64 { return &v }

var metricRegistry = map[MetricID]MetricDescriptor{
	MetricElectricalActivePower: {
		ID: MetricElectricalActivePower, DisplayName: "Active power",
		CanonicalUnit: "kW", ValueKind: ValueKindFloat64, Readable: true,
	},
	MetricElectricalReactivePower: {
		ID: MetricElectricalReactivePower, DisplayName: "Reactive power",
		CanonicalUnit: "kvar", ValueKind: ValueKindFloat64, Readable: true,
	},
	MetricElectricalActivePowerSetpoint: {
		ID: MetricElectricalActivePowerSetpoint, DisplayName: "Active power setpoint",
		CanonicalUnit: "kW", ValueKind: ValueKindFloat64, Writable: true,
	},
	MetricEnergyStorageStateOfCharge: {
		ID: MetricEnergyStorageStateOfCharge, DisplayName: "State of charge",
		CanonicalUnit: "%", ValueKind: ValueKindFloat64, Readable: true,
		MinValue: float64Ptr(0), MaxValue: float64Ptr(100),
	},
}

var capabilityRegistry = map[CapabilityID]CapabilityDescriptor{
	CapabilityEnergyStorage: {
		ID: CapabilityEnergyStorage, DisplayName: "Energy storage", SchemaVersion: 1,
	},
	CapabilityPowerGeneration: {
		ID: CapabilityPowerGeneration, DisplayName: "Power generation", SchemaVersion: 1,
	},
	CapabilityPowerConsumption: {
		ID: CapabilityPowerConsumption, DisplayName: "Power consumption", SchemaVersion: 1,
	},
	CapabilityLoadFlexibility: {
		ID: CapabilityLoadFlexibility, DisplayName: "Load flexibility", SchemaVersion: 1,
	},
	CapabilityReactivePowerControl: {
		ID: CapabilityReactivePowerControl, DisplayName: "Reactive power control", SchemaVersion: 1,
	},
	CapabilityEVCharging: {
		ID: CapabilityEVCharging, DisplayName: "EV charging", SchemaVersion: 1,
	},
	CapabilityEVVehicleToGrid: {
		ID: CapabilityEVVehicleToGrid, DisplayName: "EV vehicle-to-grid", SchemaVersion: 1,
	},
}

func LookupMetric(id MetricID) (MetricDescriptor, bool) {
	d, ok := metricRegistry[id]
	return d, ok
}

func LookupCapability(id CapabilityID) (CapabilityDescriptor, bool) {
	d, ok := capabilityRegistry[id]
	return d, ok
}

func AllMetrics() []MetricDescriptor {
	out := make([]MetricDescriptor, 0, len(metricRegistry))
	for _, descriptor := range metricRegistry {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func AllCapabilities() []CapabilityDescriptor {
	out := make([]CapabilityDescriptor, 0, len(capabilityRegistry))
	for _, descriptor := range capabilityRegistry {
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
