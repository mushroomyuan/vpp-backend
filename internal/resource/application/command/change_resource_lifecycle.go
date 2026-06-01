package command

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ChangeResourceLifecycle struct {
	TenantID   string
	ResourceID string
	Status     model.NodeLifecycleStatus
}

type ChangeResourceLifecycleHandler decorator.CommandHandler[ChangeResourceLifecycle, struct{}]

type changeResourceLifecycleHandler struct {
	nodes port.NodeRepository
}

func NewChangeResourceLifecycleHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) ChangeResourceLifecycleHandler {
	if nodes == nil {
		panic("NewChangeResourceLifecycleHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[ChangeResourceLifecycle, struct{}](
		changeResourceLifecycleHandler{nodes: nodes},
		metricClient,
	)
}

func (h changeResourceLifecycleHandler) Handle(ctx context.Context, cmd ChangeResourceLifecycle) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "change_resource_lifecycle")
	defer span.End()

	return struct{}{}, h.nodes.UpdateStatus(
		ctx,
		strings.TrimSpace(cmd.TenantID),
		strings.TrimSpace(cmd.ResourceID),
		cmd.Status,
	)
}
