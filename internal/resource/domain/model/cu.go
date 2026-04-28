package model

import "errors"

// CU (Control Unit) is a logical scheduling unit within a Resource.
// CUs can be nested via ParentCUID to model hierarchical aggregations
// (e.g. a battery rack containing multiple battery modules).
type CU struct {
	ID             string
	ResourceID     string
	ParentCUID     string // empty means this is a top-level CU under the resource
	Name           string
	Type           string
	CapabilityTags []string       // e.g. ["frequency_regulation", "peak_shaving"]
	Metadata       map[string]any // protocol, vendor model, and other extension fields
}

func NewCU(
	id, resourceID, parentCUID, name, cuType string,
	capabilityTags []string,
	metadata map[string]any,
) (*CU, error) {
	if id == "" {
		return nil, errors.New("cu id is required")
	}
	if resourceID == "" {
		return nil, errors.New("resource id is required")
	}
	if name == "" {
		return nil, errors.New("cu name is required")
	}
	return &CU{
		ID:             id,
		ResourceID:     resourceID,
		ParentCUID:     parentCUID,
		Name:           name,
		Type:           cuType,
		CapabilityTags: capabilityTags,
		Metadata:       metadata,
	}, nil
}
