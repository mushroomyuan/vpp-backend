package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

type CUCapability struct {
	ID            string
	TenantID      string
	CUID          string
	CapabilityID  contracts.CapabilityID
	SchemaVersion int
	Spec          []byte
	Enabled       bool
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type CreateCUCapabilityParams struct {
	ID            string
	TenantID      string
	CUID          string
	CapabilityID  string
	SchemaVersion int
	Spec          []byte
	Enabled       bool
}

func NewCUCapability(params CreateCUCapabilityParams) (*CUCapability, error) {
	id, err := contracts.ParseCapabilityID(strings.TrimSpace(params.CapabilityID))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	capability := &CUCapability{
		ID:            strings.TrimSpace(params.ID),
		TenantID:      strings.TrimSpace(params.TenantID),
		CUID:          strings.TrimSpace(params.CUID),
		CapabilityID:  id,
		SchemaVersion: params.SchemaVersion,
		Spec:          append([]byte(nil), params.Spec...),
		Enabled:       params.Enabled,
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := capability.Validate(); err != nil {
		return nil, err
	}
	return capability, nil
}

func (c *CUCapability) Validate() error {
	if c == nil {
		return errors.New("capability is nil")
	}
	if c.ID == "" {
		return errors.New("id is required")
	}
	if c.TenantID == "" {
		return errors.New("tenant_id is required")
	}
	if c.CUID == "" {
		return errors.New("cu_id is required")
	}
	parsed, err := contracts.ParseCapabilityID(string(c.CapabilityID))
	if err != nil {
		return err
	}
	c.CapabilityID = parsed
	if c.Version <= 0 {
		return errors.New("version must be positive")
	}
	return contracts.ValidateCapabilitySpec(c.CapabilityID, c.SchemaVersion, c.Spec)
}

func (c *CUCapability) Replace(
	schemaVersion int,
	spec []byte,
	enabled bool,
	expectedVersion int64,
) error {
	if expectedVersion > 0 && c.Version != expectedVersion {
		return fmt.Errorf("capability version conflict: have %d, expected %d", c.Version, expectedVersion)
	}
	c.SchemaVersion = schemaVersion
	c.Spec = append(c.Spec[:0], spec...)
	c.Enabled = enabled
	c.Version++
	c.UpdatedAt = time.Now()
	return c.Validate()
}
