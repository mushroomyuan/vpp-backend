package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type SiteRepository interface {
	Create(ctx context.Context, s *model.Site) (*model.Site, error)
	Update(ctx context.Context, s *model.Site) error

	FindByID(ctx context.Context, tenantID, id string) (*model.Site, error)
	List(ctx context.Context, filter SiteFilter) (*PageResult[*model.Site], error)

	// SoftDelete(ctx context.Context, tenantID, id string) error
}
