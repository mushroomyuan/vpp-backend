package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetCU struct {
	TenantID string
	ID       string
}

type GetCUHandler decorator.QueryHandler[GetCU, *model.CU]

type getCUHandler struct {
	cuRepo port.CURepository
}

func NewGetCUHandler(
	cuRepo port.CURepository,
	metricClient decorator.MetricsClient,
) GetCUHandler {
	if cuRepo == nil {
		panic("NewGetCUHandler parameter cuRepo is nil")
	}
	return decorator.ApplyQueryDecorators[GetCU, *model.CU](
		getCUHandler{cuRepo: cuRepo},
		metricClient,
	)
}

func (h getCUHandler) Handle(ctx context.Context, q GetCU) (*model.CU, error) {
	return h.cuRepo.FindByID(ctx, q.TenantID, q.ID)
}
