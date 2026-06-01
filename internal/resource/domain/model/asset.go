package model

import (
	"errors"
	"strings"
	"time"
)

// DispatchStatus is scheduling / availability for a VPP asset.
type DispatchStatus string

const (
	DispatchStatusUnknown     DispatchStatus = "unknown"
	DispatchStatusAvailable   DispatchStatus = "available"
	DispatchStatusUnavailable DispatchStatus = "unavailable"
	DispatchStatusMaintenance DispatchStatus = "maintenance"
	DispatchStatusLimited     DispatchStatus = "limited"
)

// Asset is a VPP logical dispatchable resource (e.g. BESS, PV, flexible load).
// Identity and tree placement live on Node; this row holds scheduling-specific attributes.
type Asset struct {
	Node

	DispatchStatus DispatchStatus

	RatedCapacityKW *float64

	// centralized | autonomous | semi_auto
	DispatchMode *string

	EnergyType *string // battery | pv | load | charger
	OwnerType  *string // self | third_party | customer

	MarketEnabled *bool
}

// CreateAssetParams defines required and optional fields for creating an Asset.
type CreateAssetParams struct {
	ID          string
	TenantID    string
	ParentID    *string // optional: nil if not yet placed in tree
	DisplayName string

	DispatchStatus DispatchStatus

	RatedCapacityKW *float64
	DispatchMode    *string
	EnergyType      *string
	OwnerType       *string
	SubType         *string
	Description     *string
	MarketEnabled   *bool
}

// NewAsset creates an Asset aggregate root.
func NewAsset(params CreateAssetParams) (*Asset, error) {
	if err := ValidateCreateNodeIdentity(params.ID, params.TenantID, params.DisplayName); err != nil {
		return nil, err
	}

	now := time.Now()
	dispatch := params.DispatchStatus
	if dispatch == "" {
		dispatch = DispatchStatusUnknown
	}

	a := &Asset{
		Node: Node{
			ID:              params.ID,
			TenantID:        params.TenantID,
			ParentID:        NormalizeParentIDPtr(params.ParentID),
			DisplayName:     params.DisplayName,
			Type:            NodeTypeAsset,
			SubType:         params.SubType,
			LifecycleStatus: NodeLifecycleActive,
			Description:     params.Description,
			Path:            "",
			Depth:           0,
			Metadata:        make(map[string]any),
			Version:         1,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		DispatchStatus:  dispatch,
		RatedCapacityKW: params.RatedCapacityKW,
		DispatchMode:    params.DispatchMode,
		EnergyType:      params.EnergyType,
		OwnerType:       params.OwnerType,
		MarketEnabled:   params.MarketEnabled,
	}

	if err := a.Validate(); err != nil {
		return nil, err
	}
	return a, nil
}

// Validate applies business rules for an Asset.
func (a *Asset) Validate() error {
	if a.DispatchMode != nil && strings.TrimSpace(*a.DispatchMode) == "" {
		return errors.New("dispatch_mode cannot be empty string")
	}
	if a.EnergyType != nil && strings.TrimSpace(*a.EnergyType) == "" {
		return errors.New("energy_type cannot be empty string")
	}
	if a.OwnerType != nil && strings.TrimSpace(*a.OwnerType) == "" {
		return errors.New("owner_type cannot be empty string")
	}
	if a.RatedCapacityKW != nil && *a.RatedCapacityKW < 0 {
		return errors.New("rated_capacity_kw must be non-negative")
	}
	return nil
}

// ============================================
// 业务方法 (聚合根的行为)
// ============================================

// UpdateDispatchStatus updates dispatch status and bumps version.
func (a *Asset) UpdateDispatchStatus(s DispatchStatus) {
	a.DispatchStatus = s
	a.UpdatedAt = time.Now()
	a.Version++
}

// SetRatedCapacityKW sets rated capacity (nil clears).
func (a *Asset) SetRatedCapacityKW(kw *float64) error {
	if kw != nil && *kw < 0 {
		return errors.New("rated_capacity_kw must be non-negative")
	}
	a.RatedCapacityKW = kw
	a.UpdatedAt = time.Now()
	a.Version++
	return nil
}

// SetMarketEnabled toggles market participation.
func (a *Asset) SetMarketEnabled(enabled bool) {
	a.MarketEnabled = &enabled
	a.UpdatedAt = time.Now()
	a.Version++
}
