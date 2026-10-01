package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type CUCapabilityFilter struct {
	TenantID      string
	CUID          string
	CapabilityIDs []string
	EnabledOnly   bool
}

type CUCapabilityRepository interface {
	Create(ctx context.Context, capability *model.CUCapability) error
	Update(ctx context.Context, capability *model.CUCapability) error
	FindByID(ctx context.Context, tenantID, id string) (*model.CUCapability, error)
	List(ctx context.Context, filter CUCapabilityFilter) ([]*model.CUCapability, error)
	Delete(ctx context.Context, tenantID, id string) error
}
