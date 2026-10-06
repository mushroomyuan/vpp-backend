package command

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdatePoint struct {
	ID               string
	TenantID         string
	MetricID         string
	ExternalAddress  string
	AccessMode       model.AccessMode
	Scale            float64
	Offset           float64
	Enabled          bool
	SafetyConstraint *model.PointSafetyConstraint
	ExpectedRevision int64
}

type UpdatePointHandler decorator.CommandHandler[UpdatePoint, struct{}]

type updatePointHandler struct {
	pointRepo port.PointRepository
	publisher port.ResourceEventPublisher
}

func NewUpdatePointHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) UpdatePointHandler {
	if pointRepo == nil {
		panic("NewUpdatePointHandler parameter pointRepo is nil")
	}
	return decorator.ApplyCommandDecorators[UpdatePoint, struct{}](
		updatePointHandler{pointRepo: pointRepo, publisher: publisher},
		metricClient,
	)
}

func (h updatePointHandler) Handle(ctx context.Context, cmd UpdatePoint) (struct{}, error) {
	point, err := h.pointRepo.FindByID(ctx, cmd.TenantID, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}

	if err := point.ReplaceBinding(
		cmd.MetricID, cmd.ExternalAddress, cmd.AccessMode,
		cmd.Scale, cmd.Offset, cmd.Enabled, cmd.SafetyConstraint, cmd.ExpectedRevision,
	); err != nil {
		return struct{}{}, err
	}

	if err := h.pointRepo.Update(ctx, point); err != nil {
		return struct{}{}, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypePointUpdated,
			TenantID:   cmd.TenantID,
			ResourceID: cmd.ID,
			Payload: platEvent.PointUpdatedPayload{
				PointID:  cmd.ID,
				TenantID: cmd.TenantID,
				CUID:     point.CUID,
				MetricID: cmd.MetricID,
				Revision: point.Revision,
			},
		}); pubErr != nil {
			logging.Warnf(ctx, logrus.Fields{
				"tenant_id":   cmd.TenantID,
				"resource_id": cmd.ID,
				"error":       pubErr.Error(),
			}, "failed to publish point updated event")
		}
	}

	return struct{}{}, nil
}
