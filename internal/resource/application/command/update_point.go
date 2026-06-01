package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
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
}

func NewUpdatePointHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
) UpdatePointHandler {
	if pointRepo == nil {
		panic("NewUpdatePointHandler parameter pointRepo is nil")
	}
	return decorator.ApplyCommandDecorators[UpdatePoint, struct{}](
		updatePointHandler{pointRepo: pointRepo},
		metricClient,
	)
}

func (h updatePointHandler) Handle(ctx context.Context, cmd UpdatePoint) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "update_point")
	defer span.End()

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
	return struct{}{}, nil
}
