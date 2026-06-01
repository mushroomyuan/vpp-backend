package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetPoint struct {
	ID       string
	TenantID string
}

type GetPointHandler decorator.QueryHandler[GetPoint, *PointView]

type getPointHandler struct {
	pointRepo    port.PointRepository
	pointRuntime port.PointRuntimeReader
}

func NewGetPointHandler(
	pointRepo port.PointRepository,
	pointRuntime port.PointRuntimeReader,
	metricClient decorator.MetricsClient,
) GetPointHandler {
	if pointRepo == nil {
		panic("NewGetPointHandler parameter pointRepo is nil")
	}
	if pointRuntime == nil {
		panic("NewGetPointHandler parameter pointRuntime is nil")
	}
	return decorator.ApplyQueryDecorators[GetPoint, *PointView](
		getPointHandler{pointRepo: pointRepo, pointRuntime: pointRuntime},
		metricClient,
	)
}

func (h getPointHandler) Handle(ctx context.Context, q GetPoint) (*PointView, error) {
	ctx, span := telemetry.Start(ctx, "get_point")
	defer span.End()

	point, err := h.pointRepo.FindByID(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	runtime, err := h.pointRuntime.GetPointRuntime(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	return &PointView{Point: point, Runtime: runtime}, nil
}
