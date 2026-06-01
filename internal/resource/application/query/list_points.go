package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
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
	Items      []*PointView
	TotalCount int64
	Offset     int
	Limit      int
}

type ListPointsHandler decorator.QueryHandler[ListPoints, *ListPointsResult]

type listPointsHandler struct {
	pointRepo    port.PointRepository
	pointRuntime port.PointRuntimeReader
}

func NewListPointsHandler(
	pointRepo port.PointRepository,
	pointRuntime port.PointRuntimeReader,
	metricClient decorator.MetricsClient,
) ListPointsHandler {
	if pointRepo == nil {
		panic("NewListPointsHandler parameter pointRepo is nil")
	}
	if pointRuntime == nil {
		panic("NewListPointsHandler parameter pointRuntime is nil")
	}
	return decorator.ApplyQueryDecorators[ListPoints, *ListPointsResult](
		listPointsHandler{pointRepo: pointRepo, pointRuntime: pointRuntime},
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

	page, err := h.pointRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*PointView, 0, len(page.Items))
	if len(page.Items) == 0 {
		return &ListPointsResult{
			Items:      items,
			TotalCount: page.TotalCount,
			Offset:     page.Offset,
			Limit:      page.Limit,
		}, nil
	}

	pointIDs := make([]string, len(page.Items))
	for i, point := range page.Items {
		pointIDs[i] = point.ID
	}
	runtimeByID, err := h.pointRuntime.MGetPointRuntimes(ctx, q.TenantID, pointIDs)
	if err != nil {
		return nil, err
	}

	for _, point := range page.Items {
		items = append(items, &PointView{
			Point:   point,
			Runtime: runtimeByID[point.ID],
		})
	}

	return &ListPointsResult{
		Items:      items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
