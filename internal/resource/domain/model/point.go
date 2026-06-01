package model

import (
	"errors"
	"strings"
)

// DataType enumerates the value types a point can carry.
type DataType string

const (
	DataTypeFloat DataType = "Float"
	DataTypeInt   DataType = "Int"
	DataTypeBool  DataType = "Bool"
	DataTypeEnum  DataType = "Enum"
)

// SafetyThreshold is optional structured alarm / limit metadata (legacy helpers may still use maps in persistence).
type SafetyThreshold struct {
	ThresholdType string // e.g. max_power | min_soc
	Value         float64
	Action        string // alarm | block | limit
	Severity      string // warning | error | critical
}

func (d DataType) IsValid() bool {
	switch d {
	case DataTypeFloat, DataTypeInt, DataTypeBool, DataTypeEnum:
		return true
	default:
		return false
	}
}

// Point is a normalized telemetry / control definition under a resource and CU.
type Point struct {
	ID               string
	TenantID         string
	AssetID          string
	CUID             string
	PointKey         string
	ExternalAddress  string
	DataType         DataType
	ExtConfig        map[string]any
	Description      string
	ControlFlag      bool
	IsVirtual        bool
	SafetyThresholds map[string]any
	CacheKeyAlias    string
}

// CreatePointParams defines fields for creating a Point.
type CreatePointParams struct {
	ID               string
	TenantID         string
	AssetID          string
	CUID             string
	PointKey         string
	ExternalAddress  string
	DataType         DataType
	ExtConfig        map[string]any
	Description      string
	ControlFlag      bool
	IsVirtual        bool
	SafetyThresholds map[string]any
	CacheKeyAlias    string
}

// NewPoint creates a Point aggregate root.
func NewPoint(params CreatePointParams) (*Point, error) {
	if err := validatePointParams(params); err != nil {
		return nil, err
	}
	ext := params.ExtConfig
	if ext == nil {
		ext = make(map[string]any)
	}
	th := params.SafetyThresholds
	if th == nil {
		th = make(map[string]any)
	}
	p := &Point{
		ID:               params.ID,
		TenantID:         params.TenantID,
		AssetID:          params.AssetID,
		CUID:             params.CUID,
		PointKey:         params.PointKey,
		ExternalAddress:  params.ExternalAddress,
		DataType:         params.DataType,
		ExtConfig:        ext,
		Description:      params.Description,
		ControlFlag:      params.ControlFlag,
		IsVirtual:        params.IsVirtual,
		SafetyThresholds: th,
		CacheKeyAlias:    params.CacheKeyAlias,
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func validatePointParams(params CreatePointParams) error {
	if strings.TrimSpace(params.ID) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(params.AssetID) == "" {
		return errors.New("resource_id is required")
	}
	if strings.TrimSpace(params.CUID) == "" {
		return errors.New("cu_id is required")
	}
	if strings.TrimSpace(params.PointKey) == "" {
		return errors.New("point_key is required")
	}
	if !params.DataType.IsValid() {
		return errors.New("invalid data type")
	}
	return nil
}

// Validate applies business rules.
func (p *Point) Validate() error {
	if p.DataType != "" && !p.DataType.IsValid() {
		return errors.New("invalid data type")
	}
	return nil
}

// ============================================
// 业务方法 (聚合根的行为)
// ============================================

// SetExtConfig replaces extension config.
func (p *Point) SetExtConfig(m map[string]any) {
	if m == nil {
		m = make(map[string]any)
	}
	p.ExtConfig = m
}

// SetSafetyThresholds replaces safety threshold map.
func (p *Point) SetSafetyThresholds(m map[string]any) {
	if m == nil {
		m = make(map[string]any)
	}
	p.SafetyThresholds = m
}
