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
	PointKey         string
	ExternalAddress  string
	DataType         model.DataType
	ExtConfig        map[string]any
	Description      string
	ControlFlag      bool
	IsVirtual        bool
	SafetyThresholds map[string]any
	CacheKeyAlias    string
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

	point.PointKey = cmd.PointKey
	point.ExternalAddress = cmd.ExternalAddress
	point.DataType = cmd.DataType
	point.SetExtConfig(cmd.ExtConfig)
	point.Description = cmd.Description
	point.ControlFlag = cmd.ControlFlag
	point.IsVirtual = cmd.IsVirtual
	point.SetSafetyThresholds(cmd.SafetyThresholds)
	point.CacheKeyAlias = cmd.CacheKeyAlias

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
				PointKey: cmd.PointKey,
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
