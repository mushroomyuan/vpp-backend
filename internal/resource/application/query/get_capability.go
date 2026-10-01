package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetCUCapability struct {
	TenantID string
	ID       string
}

type GetCUCapabilityHandler decorator.QueryHandler[GetCUCapability, *model.CUCapability]

type getCUCapabilityHandler struct {
	repo port.CUCapabilityRepository
}

func NewGetCUCapabilityHandler(
	repo port.CUCapabilityRepository,
	metrics decorator.MetricsClient,
) GetCUCapabilityHandler {
	if repo == nil {
		panic("NewGetCUCapabilityHandler: repo is required")
	}
	return decorator.ApplyQueryDecorators[GetCUCapability, *model.CUCapability](
		getCUCapabilityHandler{repo: repo},
		metrics,
	)
}

func (h getCUCapabilityHandler) Handle(
	ctx context.Context,
	q GetCUCapability,
) (*model.CUCapability, error) {
	return h.repo.FindByID(ctx, q.TenantID, q.ID)
}
