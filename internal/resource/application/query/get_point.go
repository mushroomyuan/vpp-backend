package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetPoint struct {
	ID       string
	TenantID string
}

type GetPointHandler decorator.QueryHandler[GetPoint, *PointView]

type getPointHandler struct {
	pointRepo port.PointRepository
}

func NewGetPointHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
) GetPointHandler {
	if pointRepo == nil {
		panic("NewGetPointHandler parameter pointRepo is nil")
	}
	return decorator.ApplyQueryDecorators[GetPoint, *PointView](
		getPointHandler{pointRepo: pointRepo},
		metricClient,
	)
}

func (h getPointHandler) Handle(ctx context.Context, q GetPoint) (*PointView, error) {
	point, err := h.pointRepo.FindByID(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	return &PointView{Point: point}, nil
}
