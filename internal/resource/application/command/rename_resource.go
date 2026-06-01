package command

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type RenameResource struct {
	TenantID   string
	ResourceID string
	NewName    string
}

type RenameResourceHandler decorator.CommandHandler[RenameResource, struct{}]

type renameResourceHandler struct {
	nodes port.NodeRepository
}

func NewRenameResourceHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) RenameResourceHandler {
	if nodes == nil {
		panic("NewRenameResourceHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[RenameResource, struct{}](
		renameResourceHandler{nodes: nodes},
		metricClient,
	)
}

func (h renameResourceHandler) Handle(ctx context.Context, cmd RenameResource) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "rename_resource")
	defer span.End()

	return struct{}{}, h.nodes.UpdateDisplayName(
		ctx,
		strings.TrimSpace(cmd.TenantID),
		strings.TrimSpace(cmd.ResourceID),
		strings.TrimSpace(cmd.NewName),
	)
}
