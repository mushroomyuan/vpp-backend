package command

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeleteOptions struct {
	IncludeDescendants bool
}

type DeleteResource struct {
	TenantID   string
	ID         string
	ResourceID string
	Opts       DeleteOptions
}

type DeleteResourceHandler decorator.CommandHandler[DeleteResource, struct{}]

type deleteResourceHandler struct {
	nodes port.NodeRepository
}

func NewDeleteResourceHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) DeleteResourceHandler {
	if nodes == nil {
		panic("NewDeleteResourceHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[DeleteResource, struct{}](
		deleteResourceHandler{nodes: nodes},
		metricClient,
	)
}

func (h deleteResourceHandler) Handle(ctx context.Context, cmd DeleteResource) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "delete_resource")
	defer span.End()

	tenantID := strings.TrimSpace(cmd.TenantID)
	resourceID := strings.TrimSpace(cmd.ResourceID)
	if resourceID == "" {
		resourceID = strings.TrimSpace(cmd.ID)
	}
	if cmd.Opts.IncludeDescendants {
		return struct{}{}, h.nodes.SoftDeleteSubtree(ctx, tenantID, resourceID)
	}
	return struct{}{}, h.nodes.SoftDelete(ctx, tenantID, resourceID)
}
