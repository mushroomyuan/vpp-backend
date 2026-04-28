package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListResources struct {
	TenantID string
	SiteID   string
	IDs      []string
	Types    []string
	NameLike string
	Offset   int
	Limit    int
}

type ListResourcesResult struct {
	Resources []*model.Resource
}

type ListResourcesHandler decorator.QueryHandler[ListResources, *ListResourcesResult]

type listResourcesHandler struct {
	resourceRepo port.ResourceRepository
}

func NewListResourcesHandler(
	resourceRepo port.ResourceRepository,
	metricClient decorator.MetricsClient,
) ListResourcesHandler {
	if resourceRepo == nil {
		panic("NewListResourcesHandler parameter resourceRepo is nil")
	}
	return decorator.ApplyQueryDecorators[ListResources, *ListResourcesResult](
		listResourcesHandler{resourceRepo: resourceRepo},
		metricClient,
	)
}

func (h listResourcesHandler) Handle(ctx context.Context, q ListResources) (*ListResourcesResult, error) {
	ctx, span := telemetry.Start(ctx, "list_resources")
	defer span.End()

	filter := port.ResourceFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		SiteID:   q.SiteID,
		IDs:      q.IDs,
		Types:    q.Types,
		NameLike: q.NameLike,
	}

	resources, err := h.resourceRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListResourcesResult{Resources: resources}, nil
}
