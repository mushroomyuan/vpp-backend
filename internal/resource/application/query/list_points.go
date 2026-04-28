package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListPoints struct {
	TenantID  string
	SiteID    string
	CUID      string
	PointKeys []string
	IsVirtual *bool
	DataTypes []string
	IDs       []string
	Offset    int
	Limit     int
}

type ListPointsResult struct {
	Points []*model.Point
}

type ListPointsHandler decorator.QueryHandler[ListPoints, *ListPointsResult]

type listPointsHandler struct {
	pointRepo port.PointRepository
}

func NewListPointsHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
) ListPointsHandler {
	if pointRepo == nil {
		panic("NewListPointsHandler parameter pointRepo is nil")
	}
	return decorator.ApplyQueryDecorators[ListPoints, *ListPointsResult](
		listPointsHandler{pointRepo: pointRepo},
		metricClient,
	)
}

func (h listPointsHandler) Handle(ctx context.Context, q ListPoints) (*ListPointsResult, error) {
	ctx, span := telemetry.Start(ctx, "list_points")
	defer span.End()

	filter := port.PointFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		SiteID:    q.SiteID,
		CUID:      q.CUID,
		PointKeys: q.PointKeys,
		IsVirtual: q.IsVirtual,
		DataTypes: q.DataTypes,
		IDs:       q.IDs,
	}

	points, err := h.pointRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListPointsResult{Points: points}, nil
}
