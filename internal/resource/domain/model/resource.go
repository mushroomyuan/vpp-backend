package model

import "errors"

// Resource represents a physical energy device (e.g. transformer, PV inverter, BESS)
// installed at a site. It is the parent of one or more CUs.
type Resource struct {
	ID           string
	TenantID     string
	SiteID       string
	Name         string
	Type         string
	Capacity     float64 // rated power in kW
	Manufacturer string
	Model        string
	Metadata     map[string]any // topology, protocol type, and other extension fields
}

func NewResource(
	id, tenantID, siteID, name, resourceType string,
	capacity float64,
	manufacturer, model string,
	metadata map[string]any,
) (*Resource, error) {
	if id == "" {
		return nil, errors.New("resource id is required")
	}
	if tenantID == "" {
		return nil, errors.New("tenant id is required")
	}
	if siteID == "" {
		return nil, errors.New("site id is required")
	}
	if name == "" {
		return nil, errors.New("resource name is required")
	}
	if resourceType == "" {
		return nil, errors.New("resource type is required")
	}
	return &Resource{
		ID:           id,
		TenantID:     tenantID,
		SiteID:       siteID,
		Name:         name,
		Type:         resourceType,
		Capacity:     capacity,
		Manufacturer: manufacturer,
		Model:        model,
		Metadata:     metadata,
	}, nil
}
