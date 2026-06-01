package command

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type MoveResource struct {
	TenantID    string
	ResourceID  string
	NewParentID string
}

type MoveResourceHandler decorator.CommandHandler[MoveResource, struct{}]

type moveResourceHandler struct {
	nodes port.NodeRepository
}

func NewMoveResourceHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) MoveResourceHandler {
	if nodes == nil {
		panic("NewMoveResourceHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[MoveResource, struct{}](
		moveResourceHandler{nodes: nodes},
		metricClient,
	)
}

func (h moveResourceHandler) Handle(ctx context.Context, cmd MoveResource) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "move_resource")
	defer span.End()

	return struct{}{}, h.nodes.Move(
		ctx,
		strings.TrimSpace(cmd.TenantID),
		strings.TrimSpace(cmd.ResourceID),
		strings.TrimSpace(cmd.NewParentID),
	)
}
