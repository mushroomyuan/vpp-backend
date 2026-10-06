package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListPoints struct {
	TenantID    string
	SiteID      string
	CUID        string
	MetricIDs   []string
	AccessModes []string
	Enabled     *bool
	IDs         []string
	Offset      int
	Limit       int
}

type ListPointsResult struct {
	Items      []*model.Point
	TotalCount int64
	Offset     int
	Limit      int
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
	filter := port.PointFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		SiteID: q.SiteID, CUID: q.CUID,
		MetricIDs: q.MetricIDs, AccessModes: q.AccessModes,
		Enabled: q.Enabled, IDs: q.IDs,
	}

	page, err := h.pointRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListPointsResult{
		Items:      page.Items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
