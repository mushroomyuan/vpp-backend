package command

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type RenameResource struct {
	TenantID   string
	ResourceID string
	NewName    string
}

type RenameResourceHandler decorator.CommandHandler[RenameResource, struct{}]

type renameResourceHandler struct {
	nodes     port.NodeRepository
	publisher port.ResourceEventPublisher
}

func NewRenameResourceHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) RenameResourceHandler {
	if nodes == nil {
		panic("NewRenameResourceHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[RenameResource, struct{}](
		renameResourceHandler{nodes: nodes, publisher: publisher},
		metricClient,
	)
}

func (h renameResourceHandler) Handle(ctx context.Context, cmd RenameResource) (struct{}, error) {
	tenantID := strings.TrimSpace(cmd.TenantID)
	resourceID := strings.TrimSpace(cmd.ResourceID)
	newName := strings.TrimSpace(cmd.NewName)

	if err := h.nodes.UpdateDisplayName(ctx, tenantID, resourceID, newName); err != nil {
		return struct{}{}, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeResourceRenamed,
			TenantID:   tenantID,
			ResourceID: resourceID,
			Payload: platEvent.ResourceRenamedPayload{
				ResourceID: resourceID,
				TenantID:   tenantID,
				NewName:    newName,
			},
		}); pubErr != nil {
			logging.Warnf(ctx, logrus.Fields{
				"tenant_id":   tenantID,
				"resource_id": resourceID,
				"error":       pubErr.Error(),
			}, "failed to publish resource renamed event")
		}
	}

	return struct{}{}, nil
}
