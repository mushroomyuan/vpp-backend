package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
)

// MappingRepository persists DeviceMapping records in the gateway's local database.
// Gateway is the sole owner of this data; no other service reads it directly.
//
// All write operations are keyed by (tenantID, id).
// Lookups are provided in two directions:
//   - external→internal: GetByExternalID is used on the telemetry ingestion path.
//   - internal→external: GetByCUCode is used on the command dispatch path.
type MappingRepository interface {
	// Create persists a new mapping. Returns domain.ErrMappingConflict if the
	// (tenant_id, external_system, external_id) combination already exists.
	Create(ctx context.Context, m *model.DeviceMapping) error

	// Delete permanently removes a mapping.
	Delete(ctx context.Context, tenantID, id string) error

	// Disable sets the mapping status to disabled without deleting it.
	// Returns domain.ErrMappingNotFound if the mapping does not exist.
	Disable(ctx context.Context, tenantID, id string) error

	// GetByExternalID looks up a mapping by the external system's device identifier.
	// Returns domain.ErrMappingNotFound when no matching row exists.
	// The caller is responsible for checking DeviceMapping.IsActive().
	GetByExternalID(ctx context.Context, tenantID, externalSystem, externalID string) (*model.DeviceMapping, error)

	// GetByCUCode looks up a mapping by the internal CUCode.
	// When a CU is reachable from multiple external systems this returns the
	// first active mapping found; behaviour is deterministic only if each CU
	// is mapped to at most one external system per tenant.
	// Returns domain.ErrMappingNotFound when no matching row exists.
	GetByCUCode(ctx context.Context, tenantID, cuCode string) (*model.DeviceMapping, error)

	// List returns all mappings for a tenant, regardless of status.
	List(ctx context.Context, tenantID string) ([]*model.DeviceMapping, error)
}
