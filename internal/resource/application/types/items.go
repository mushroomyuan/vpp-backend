package types

import (
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

// AssetItem is the per-record input for batch asset import.
type AssetItem struct {
	Name            string
	DispatchStatus  model.DispatchStatus
	SubType         *string
	RatedCapacityKW *float64
	DispatchMode    *string
	EnergyType      *string
	OwnerType       *string
	Description     *string
	MarketEnabled   *bool
	Metadata        map[string]any
}

func (a AssetItem) Validate() error {
	if a.Name == "" {
		return errors.New("Name is required")
	}
	if a.SubType != nil && *a.SubType == "" {
		return errors.New("SubType cannot be empty string")
	}
	if a.DispatchMode != nil && *a.DispatchMode == "" {
		return errors.New("DispatchMode cannot be empty string")
	}
	if a.EnergyType != nil && *a.EnergyType == "" {
		return errors.New("EnergyType cannot be empty string")
	}
	if a.OwnerType != nil && *a.OwnerType == "" {
		return errors.New("OwnerType cannot be empty string")
	}
	if a.Description != nil && *a.Description == "" {
		return errors.New("Description cannot be empty string")
	}
	if a.RatedCapacityKW != nil && *a.RatedCapacityKW < 0 {
		return errors.New("RatedCapacityKW must be non-negative")
	}
	return nil
}

// CUItem is the per-record input for batch CU creation under a Resource.
type CUItem struct {
	ParentID       *string
	Name           string
	Type           string
	Provider       *string
	ExternalID     *string
	Protocol       *string
	ProtocolConfig map[string]any
	Connection     *model.ConnectionConfig
	CapabilityTags []string
	Description    *string
	Metadata       map[string]any
}

func (c CUItem) Validate() error {
	var missing []string
	if c.Name == "" {
		missing = append(missing, "Name")
	}
	if c.Type == "" {
		missing = append(missing, "Type")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %v", missing)
	}
	return nil
}

// PointItem is the per-record input for batch point creation under a CU.
type PointItem struct {
	PointKey         string
	ExternalAddress  string
	DataType         model.DataType
	ExtConfig        map[string]any
	Description      string
	ControlFlag      bool
	IsVirtual        bool
	SafetyThresholds map[string]any
	CacheKeyAlias    string
}

func (p PointItem) Validate() error {
	if p.PointKey == "" {
		return errors.New("PointKey is required")
	}
	if !p.DataType.IsValid() {
		return errors.New("invalid DataType")
	}
	return nil
}
