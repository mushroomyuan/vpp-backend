package model

import "errors"

// SiteStatus represents the operational state of a site.
type SiteStatus int8

const (
	SiteStatusUnknown           SiteStatus = 0
	SiteStatusUnderConstruction SiteStatus = 1
	SiteStatusOperating         SiteStatus = 2
	SiteStatusFault             SiteStatus = 3
	SiteStatusOffline           SiteStatus = 4
)

func (s SiteStatus) String() string {
	switch s {
	case SiteStatusUnderConstruction:
		return "under_construction"
	case SiteStatusOperating:
		return "operating"
	case SiteStatusFault:
		return "fault"
	case SiteStatusOffline:
		return "offline"
	default:
		return "unknown"
	}
}

// Location is a value object that captures the geographic position of a site.
type Location struct {
	Latitude  float64
	Longitude float64
	Address   string
}

// Site represents a physical or logical energy station belonging to a project.
type Site struct {
	ID          string
	TenantID    string
	Name        string
	Location    Location
	Description string
	Status      SiteStatus
}

func NewSite(id, tenantID, name, description string, location Location, status SiteStatus) (*Site, error) {
	if id == "" {
		return nil, errors.New("site id is required")
	}
	if tenantID == "" {
		return nil, errors.New("tenant id is required")
	}
	if name == "" {
		return nil, errors.New("site name is required")
	}
	return &Site{
		ID:          id,
		TenantID:    tenantID,
		Name:        name,
		Location:    location,
		Description: description,
		Status:      status,
	}, nil
}

func NewSiteUnderConstruction(id, tenantID, name, description string, location Location) (*Site, error) {
	if id == "" {
		return nil, errors.New("site id is required")
	}
	if tenantID == "" {
		return nil, errors.New("tenant id is required")
	}
	if name == "" {
		return nil, errors.New("site name is required")
	}
	return &Site{
		ID:          id,
		TenantID:    tenantID,
		Name:        name,
		Location:    location,
		Description: description,
		Status:      SiteStatusUnderConstruction,
	}, nil
}
