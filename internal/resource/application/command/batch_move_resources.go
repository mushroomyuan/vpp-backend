package command

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type BatchMoveResources struct {
	TenantID     string
	ResourceIDs  []string
	NewParentID  string
}

type BatchMoveResourcesResult struct {
	MovedCount int
}

type BatchMoveResourcesHandler decorator.CommandHandler[BatchMoveResources, *BatchMoveResourcesResult]

type batchMoveResourcesHandler struct {
	nodes port.NodeRepository
}

func NewBatchMoveResourcesHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) BatchMoveResourcesHandler {
	if nodes == nil {
		panic("NewBatchMoveResourcesHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[BatchMoveResources, *BatchMoveResourcesResult](
		batchMoveResourcesHandler{nodes: nodes},
		metricClient,
	)
}

func (h batchMoveResourcesHandler) Handle(ctx context.Context, cmd BatchMoveResources) (*BatchMoveResourcesResult, error) {
	ctx, span := telemetry.Start(ctx, "batch_move_resources")
	defer span.End()

	tenantID := strings.TrimSpace(cmd.TenantID)
	newParentID := strings.TrimSpace(cmd.NewParentID)

	moved := 0
	for _, id := range cmd.ResourceIDs {
		resourceID := strings.TrimSpace(id)
		if resourceID == "" {
			continue
		}
		if err := h.nodes.Move(ctx, tenantID, resourceID, newParentID); err != nil {
			return nil, err
		}
		moved++
	}
	return &BatchMoveResourcesResult{MovedCount: moved}, nil
}
