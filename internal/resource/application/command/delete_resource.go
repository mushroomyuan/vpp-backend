package command

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
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
	nodes     port.NodeRepository
	publisher port.ResourceEventPublisher
}

func NewDeleteResourceHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) DeleteResourceHandler {
	if nodes == nil {
		panic("NewDeleteResourceHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[DeleteResource, struct{}](
		deleteResourceHandler{nodes: nodes, publisher: publisher},
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

	var deleteErr error
	if cmd.Opts.IncludeDescendants {
		deleteErr = h.nodes.SoftDeleteSubtree(ctx, tenantID, resourceID)
	} else {
		deleteErr = h.nodes.SoftDelete(ctx, tenantID, resourceID)
	}
	if deleteErr != nil {
		return struct{}{}, deleteErr
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeResourceDeleted,
			TenantID:   tenantID,
			ResourceID: resourceID,
			Payload: platEvent.ResourceDeletedPayload{
				ResourceID:         resourceID,
				TenantID:           tenantID,
				IncludeDescendants: cmd.Opts.IncludeDescendants,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish resource deleted event")
		}
	}

	return struct{}{}, nil
}
