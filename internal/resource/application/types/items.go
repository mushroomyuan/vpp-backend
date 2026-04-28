package types

import (
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

// ResourceItem is the per-record input for batch import.
type ResourceItem struct {
	Name         string
	Type         string
	Capacity     float64
	Manufacturer string
	Model        string
	Metadata     map[string]any
}

func (r ResourceItem) Validate() error {
	var missing []string
	if r.Name == "" {
		missing = append(missing, "Name")
	}
	if r.Type == "" {
		missing = append(missing, "Type")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required fields: %v", missing)
	}
	return nil
}

// CUItem is the per-record input for batch CU creation under a Resource.
type CUItem struct {
	ParentCUID     string
	Name           string
	Type           string
	CapabilityTags []string
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
