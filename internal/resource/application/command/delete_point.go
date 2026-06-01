package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeletePoint struct {
	TenantID string
	ID       string
}

type DeletePointHandler decorator.CommandHandler[DeletePoint, struct{}]

type deletePointHandler struct {
	pointRepo port.PointRepository
}

func NewDeletePointHandler(
	pointRepo port.PointRepository,
	metricClient decorator.MetricsClient,
) DeletePointHandler {
	if pointRepo == nil {
		panic("NewDeletePointHandler parameter pointRepo is nil")
	}
	return decorator.ApplyCommandDecorators[DeletePoint, struct{}](
		deletePointHandler{pointRepo: pointRepo},
		metricClient,
	)
}

func (h deletePointHandler) Handle(ctx context.Context, cmd DeletePoint) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "delete_point")
	defer span.End()

	return struct{}{}, h.pointRepo.SoftDelete(ctx, cmd.TenantID, cmd.ID)
}
