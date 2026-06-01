package model

import "time"

// AssetRuntime is hot scheduling state for an asset (e.g. Redis asset:{id}:runtime).
type AssetRuntime struct {
	AssetID  string
	TenantID string

	Online bool

	CurrentPowerKW        *float64
	AvailablePowerKW      *float64
	SOC                   *float64
	Dispatchable          bool
	NotDispatchableReason *string
	MaxChargePowerKW      *float64
	MaxDischargePowerKW   *float64

	UpdatedAt time.Time
}

// CURuntime is connection-plane runtime for a control unit (e.g. Redis cu:{id}:runtime).
type CURuntime struct {
	CUID     string
	TenantID string

	ConnStatus string

	LastSeenAt time.Time
	LatencyMS  *int64
	LastError  *string

	UpdatedAt time.Time
}

// PointRuntime is latest cached value for a point (e.g. Redis point:{id}:runtime).
type PointRuntime struct {
	PointID  string
	TenantID string

	Value         *string
	NumericValue  *float64
	QualityStatus *string
	Sequence      int64

	SampledAt time.Time
	UpdatedAt time.Time
}
