package domain

import "time"

// PointDef is a telemetry/control template loaded from Resource Point.
type PointDef struct {
	ID          string
	PointKey    string
	DataType    string
	ControlFlag bool
	IsVirtual   bool
}

// PointValue is one metric sample produced by Device.Snapshot().
type PointValue struct {
	PointKey string
	Value    float64
}

// DeviceStatus is the runtime connectivity / health of a simulated CU.
type DeviceStatus string

const (
	StatusOnline  DeviceStatus = "online"
	StatusOffline DeviceStatus = "offline"
	StatusFault   DeviceStatus = "fault"
)

// DeviceSpec carries Resource-derived configuration used to construct a Device.
type DeviceSpec struct {
	TenantID        string
	CUCode          string
	ExternalID      string
	Name            string
	Type            string
	Provider        string
	AssetID         string
	RatedCapacityKW float64
	EnergyType      string
	Points          []PointDef
}

// Device is a live in-memory CU runtime.
type Device interface {
	CUCode() string
	ExternalID() string
	Type() string
	Name() string
	Tick(delta time.Duration)
	Execute(pointKey string, value float64) error
	Snapshot() []PointValue
	Status() DeviceStatus
	SetStatus(status DeviceStatus)
	Reset()
}

// FaultKind enumerates injectable failure modes.
type FaultKind string

const (
	FaultOffline         FaultKind = "offline"
	FaultCommandReject   FaultKind = "command_reject"
	FaultTelemetryDelay  FaultKind = "telemetry_delay"
	FaultClear           FaultKind = "clear"
)
