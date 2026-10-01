package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeleteCUCapability struct {
	TenantID string
	ID       string
}

type DeleteCUCapabilityHandler decorator.CommandHandler[DeleteCUCapability, struct{}]

type deleteCUCapabilityHandler struct {
	repo port.CUCapabilityRepository
}

func NewDeleteCUCapabilityHandler(
	repo port.CUCapabilityRepository,
	metrics decorator.MetricsClient,
) DeleteCUCapabilityHandler {
	if repo == nil {
		panic("NewDeleteCUCapabilityHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[DeleteCUCapability, struct{}](
		deleteCUCapabilityHandler{repo: repo},
		metrics,
	)
}

func (h deleteCUCapabilityHandler) Handle(
	ctx context.Context,
	cmd DeleteCUCapability,
) (struct{}, error) {
	return struct{}{}, h.repo.Delete(ctx, cmd.TenantID, cmd.ID)
}
