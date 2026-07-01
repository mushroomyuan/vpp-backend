package domain

import "errors"

var (
	// ErrMappingNotFound is returned when no DeviceMapping matches the lookup key.
	ErrMappingNotFound = errors.New("device mapping not found")

	// ErrMappingDisabled is returned when a mapping exists but its status is disabled.
	// Callers should treat this as a rejection, not a retry trigger.
	ErrMappingDisabled = errors.New("device mapping is disabled")

	// ErrMappingConflict is returned when a Create would violate the unique constraint
	// (tenant_id, external_system, external_id).
	ErrMappingConflict = errors.New("device mapping already exists for this external ID")
)
