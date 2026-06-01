package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type AssetRepository interface {
	Create(ctx context.Context, Asset *model.Asset) (*model.Asset, error)
	BatchCreate(ctx context.Context, Assets []*model.Asset) error
	Update(ctx context.Context, Asset *model.Asset) error
	FindByID(ctx context.Context, tenantID, id string) (*model.Asset, error)
	List(ctx context.Context, f AssetFilter) (*PageResult[*model.Asset], error)
	// SoftDelete(ctx context.Context, tenantID, id string) error
	// BatchDelete(ctx context.Context, tenantID string, ids []string) error
}
