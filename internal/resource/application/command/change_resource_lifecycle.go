package command

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
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
	nodes     port.NodeRepository
	publisher port.ResourceEventPublisher
}

func NewChangeResourceLifecycleHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) ChangeResourceLifecycleHandler {
	if nodes == nil {
		panic("NewChangeResourceLifecycleHandler parameter nodes is nil")
	}
	return decorator.ApplyCommandDecorators[ChangeResourceLifecycle, struct{}](
		changeResourceLifecycleHandler{nodes: nodes, publisher: publisher},
		metricClient,
	)
}

func (h changeResourceLifecycleHandler) Handle(ctx context.Context, cmd ChangeResourceLifecycle) (struct{}, error) {
	tenantID := strings.TrimSpace(cmd.TenantID)
	resourceID := strings.TrimSpace(cmd.ResourceID)

	if err := h.nodes.UpdateStatus(ctx, tenantID, resourceID, cmd.Status); err != nil {
		return struct{}{}, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeLifecycleChanged,
			TenantID:   tenantID,
			ResourceID: resourceID,
			Payload: platEvent.LifecycleChangedPayload{
				ResourceID: resourceID,
				TenantID:   tenantID,
				Status:     string(cmd.Status),
			},
		}); pubErr != nil {
			logging.Warnf(ctx, logrus.Fields{
				"tenant_id":   tenantID,
				"resource_id": resourceID,
				"error":       pubErr.Error(),
			}, "failed to publish lifecycle changed event")
		}
	}

	return struct{}{}, nil
}
