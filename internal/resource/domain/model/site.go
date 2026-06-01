package model

import (
	"errors"
	"strings"
	"time"
)

// OperatingStatus matches persisted site operating state (proto OperatingStatus).
type OperatingStatus int8

const (
	OperatingStatusUnknown           OperatingStatus = 0
	OperatingStatusUnderConstruction OperatingStatus = 1
	OperatingStatusOperating         OperatingStatus = 2
	OperatingStatusFault             OperatingStatus = 3
	OperatingStatusOffline           OperatingStatus = 4
)

// Location is physical site detail (stored as JSON on the site row).
type Location struct {
	Address   string
	Latitude  float64
	Longitude float64
}

// Site is the extension row for a station / park / building / charging site.
// Display name and tree fields live on Node.
type Site struct {
	Node

	OperatingStatus OperatingStatus

	Location *Location
}

// CreateSiteParams defines fields for creating a Site.
type CreateSiteParams struct {
	ID       string
	TenantID string
	ParentID *string // nil for root site

	DisplayName     string
	Description     *string
	Location        *Location
	OperatingStatus OperatingStatus
	SubType         *string
}

// NewSite creates a Site aggregate root.
func NewSite(params CreateSiteParams) (*Site, error) {
	if err := ValidateCreateNodeIdentity(params.ID, params.TenantID, params.DisplayName); err != nil {
		return nil, err
	}

	now := time.Now()
	status := params.OperatingStatus
	if status == 0 {
		status = OperatingStatusUnknown
	}

	s := &Site{
		Node: Node{
			ID:              params.ID,
			TenantID:        params.TenantID,
			ParentID:        NormalizeParentIDPtr(params.ParentID),
			DisplayName:     params.DisplayName,
			Type:            NodeTypeSite,
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
		OperatingStatus: status,
		Location:        params.Location,
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// NewSiteUnderConstruction creates a site in under-construction status (command / API default).
func NewSiteUnderConstruction(id, tenantID, displayName, description string, loc Location) (*Site, error) {
	var desc *string
	if trimmed := strings.TrimSpace(description); trimmed != "" {
		desc = &trimmed
	}
	var locPtr *Location
	if loc != (Location{}) {
		l := loc
		locPtr = &l
	}
	return NewSite(CreateSiteParams{
		ID:              id,
		TenantID:        tenantID,
		ParentID:        nil,
		DisplayName:     displayName,
		Description:     desc,
		Location:        locPtr,
		OperatingStatus: OperatingStatusUnderConstruction,
	})
}

// Validate checks site status and location are coherent.
func (s *Site) Validate() error {
	switch s.OperatingStatus {
	case OperatingStatusUnknown, OperatingStatusUnderConstruction, OperatingStatusOperating, OperatingStatusFault, OperatingStatusOffline:
		return nil
	default:
		return errors.New("invalid site status")
	}
}

// ============================================
// 业务方法 (聚合根的行为)
// ============================================

// SetOperatingStatus updates persisted operating status (proto OperatingStatus).
func (s *Site) SetOperatingStatus(st OperatingStatus) error {
	switch st {
	case OperatingStatusUnknown, OperatingStatusUnderConstruction, OperatingStatusOperating, OperatingStatusFault, OperatingStatusOffline:
		s.OperatingStatus = st
		s.UpdatedAt = time.Now()
		s.Version++
		return nil
	default:
		return errors.New("invalid site status")
	}
}

// SetLocation replaces site location.
func (s *Site) SetLocation(loc *Location) {
	s.Location = loc
	s.UpdatedAt = time.Now()
	s.Version++
}
