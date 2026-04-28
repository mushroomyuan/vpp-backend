package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeleteCU struct {
	ID string
}

type DeleteCUHandler decorator.CommandHandler[DeleteCU, struct{}]

type deleteCUHandler struct {
	cuRepo port.CURepository
}

func NewDeleteCUHandler(
	cuRepo port.CURepository,
	metricClient decorator.MetricsClient,
) DeleteCUHandler {
	if cuRepo == nil {
		panic("NewDeleteCUHandler parameter cuRepo is nil")
	}
	return decorator.ApplyCommandDecorators[DeleteCU, struct{}](
		deleteCUHandler{cuRepo: cuRepo},
		metricClient,
	)
}

func (h deleteCUHandler) Handle(ctx context.Context, cmd DeleteCU) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "delete_cu")
	defer span.End()

	return struct{}{}, h.cuRepo.SoftDelete(ctx, cmd.ID)
}
