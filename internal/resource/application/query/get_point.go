package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetPoint struct {
	ID string
}

type GetPointHandler decorator.QueryHandler[GetPoint, *model.Point]

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
	return decorator.ApplyQueryDecorators[GetPoint, *model.Point](
		getPointHandler{pointRepo: pointRepo},
		metricClient,
	)
}

func (h getPointHandler) Handle(ctx context.Context, q GetPoint) (*model.Point, error) {
	ctx, span := telemetry.Start(ctx, "get_point")
	defer span.End()

	return h.pointRepo.FindByID(ctx, q.ID)
}
