package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetResource struct {
	TenantID string
	ID       string
}

type GetResourceHandler decorator.QueryHandler[GetResource, *model.Resource]

type getResourceHandler struct {
	resourceRepo port.ResourceRepository
}

func NewGetResourceHandler(
	resourceRepo port.ResourceRepository,
	metricClient decorator.MetricsClient,
) GetResourceHandler {
	if resourceRepo == nil {
		panic("NewGetResourceHandler parameter resourceRepo is nil")
	}
	return decorator.ApplyQueryDecorators[GetResource, *model.Resource](
		getResourceHandler{resourceRepo: resourceRepo},
		metricClient,
	)
}

func (h getResourceHandler) Handle(ctx context.Context, q GetResource) (*model.Resource, error) {
	ctx, span := telemetry.Start(ctx, "get_resource")
	defer span.End()

	return h.resourceRepo.FindByID(ctx, q.TenantID, q.ID)
}
