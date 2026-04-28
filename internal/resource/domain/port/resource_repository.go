package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type ResourceRepository interface {
	Create(ctx context.Context, resource *model.Resource) (*model.Resource, error)
	BatchCreate(ctx context.Context, resources []*model.Resource) error
	Update(ctx context.Context, resource *model.Resource) error
	FindByID(ctx context.Context, tenantID, id string) (*model.Resource, error)
	List(ctx context.Context, f ResourceFilter) ([]*model.Resource, error)
	SoftDelete(ctx context.Context, tenantID, id string) error
	BatchDelete(ctx context.Context, tenantID string, ids []string) error
}
