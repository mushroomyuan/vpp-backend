package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeleteResource struct {
	TenantID string
	ID       string
}

type DeleteResourceHandler decorator.CommandHandler[DeleteResource, struct{}]

type deleteResourceHandler struct {
	resourceRepo port.ResourceRepository
}

func NewDeleteResourceHandler(
	resourceRepo port.ResourceRepository,
	metricClient decorator.MetricsClient,
) DeleteResourceHandler {
	if resourceRepo == nil {
		panic("NewDeleteResourceHandler parameter resourceRepo is nil")
	}
	return decorator.ApplyCommandDecorators[DeleteResource, struct{}](
		deleteResourceHandler{resourceRepo: resourceRepo},
		metricClient,
	)
}

func (h deleteResourceHandler) Handle(ctx context.Context, cmd DeleteResource) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "delete_resource")
	defer span.End()

	return struct{}{}, h.resourceRepo.SoftDelete(ctx, cmd.TenantID, cmd.ID)
}
