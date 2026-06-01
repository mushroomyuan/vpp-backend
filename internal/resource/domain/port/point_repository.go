package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

type PointRepository interface {
	Create(ctx context.Context, p *model.Point) (*model.Point, error)
	BatchCreate(ctx context.Context, points []*model.Point) error
	Update(ctx context.Context, p *model.Point) error

	FindByID(ctx context.Context, tenantID, id string) (*model.Point, error)
	List(ctx context.Context, filter PointFilter) (*PageResult[*model.Point], error)

	SoftDelete(ctx context.Context, tenantID, id string) error
	BatchDelete(ctx context.Context, tenantID string, ids []string) error
}
