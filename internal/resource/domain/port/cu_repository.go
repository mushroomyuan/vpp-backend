package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type CURepository interface {
	Create(ctx context.Context, cu *model.CU) (*model.CU, error)
	BatchCreate(ctx context.Context, cus []*model.CU) error
	Update(ctx context.Context, cu *model.CU) error

	FindByID(ctx context.Context, tenantID, id string) (*model.CU, error)
	List(ctx context.Context, filter CUFilter) (*PageResult[*model.CU], error)

	// SoftDelete(ctx context.Context,tenantID, id string) error
	// BatchDelete(ctx context.Context, tenantID string, ids []string) error
}
