package policy

import (
	"context"
	"time"
)

// ListFilter selects live policies for one tenant.
type ListFilter struct {
	TenantID    string
	EnabledOnly bool
}

// Repository persists policies. Implementations hide the SOC spec table.
type Repository interface {
	Create(ctx context.Context, p *Policy) error
	// Update writes p when the stored version is still expectedVersion.
	// p.Version must be expectedVersion + 1.
	Update(ctx context.Context, p *Policy, expectedVersion int64) error
	SoftDelete(ctx context.Context, tenantID, id, actor string, at time.Time) error
	FindByID(ctx context.Context, tenantID, id string) (*Policy, error)
	List(ctx context.Context, filter ListFilter) ([]*Policy, error)
	// ListEnabled returns every live enabled policy. The decision loop uses it.
	// Tenant-facing APIs keep using List.
	ListEnabled(ctx context.Context) ([]*Policy, error)
}
