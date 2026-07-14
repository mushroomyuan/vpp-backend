package command

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeletePoint struct {
	TenantID string
	ID       string
}

type DeletePointHandler decorator.CommandHandler[DeletePoint, struct{}]

type deletePointHandler struct {
	pointRepo port.PointRepository
	publisher port.ResourceEventPublisher
}

func NewDeletePointHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) DeletePointHandler {
	if pointRepo == nil {
		panic("NewDeletePointHandler parameter pointRepo is nil")
	}
	return decorator.ApplyCommandDecorators[DeletePoint, struct{}](
		deletePointHandler{pointRepo: pointRepo, publisher: publisher},
		metricClient,
	)
}

func (h deletePointHandler) Handle(ctx context.Context, cmd DeletePoint) (struct{}, error) {
	if err := h.pointRepo.SoftDelete(ctx, cmd.TenantID, cmd.ID); err != nil {
		return struct{}{}, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypePointDeleted,
			TenantID:   cmd.TenantID,
			ResourceID: cmd.ID,
			Payload: platEvent.PointDeletedPayload{
				PointID:  cmd.ID,
				TenantID: cmd.TenantID,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish point deleted event")
		}
	}

	return struct{}{}, nil
}
