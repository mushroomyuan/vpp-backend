package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetCU struct {
	TenantID string
	ID       string
}

type GetCUHandler decorator.QueryHandler[GetCU, *CUView]

type getCUHandler struct {
	cuRepo    port.CURepository
	cuRuntime port.CURuntimeReader
}

func NewGetCUHandler(
	cuRepo port.CURepository,
	cuRuntime port.CURuntimeReader,
	metricClient decorator.MetricsClient,
) GetCUHandler {
	if cuRepo == nil {
		panic("NewGetCUHandler parameter cuRepo is nil")
	}
	if cuRuntime == nil {
		panic("NewGetCUHandler parameter cuRuntime is nil")
	}
	return decorator.ApplyQueryDecorators[GetCU, *CUView](
		getCUHandler{cuRepo: cuRepo, cuRuntime: cuRuntime},
		metricClient,
	)
}

func (h getCUHandler) Handle(ctx context.Context, q GetCU) (*CUView, error) {
	ctx, span := telemetry.Start(ctx, "get_cu")
	defer span.End()

	cu, err := h.cuRepo.FindByID(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	runtime, err := h.cuRuntime.GetCURuntime(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	return &CUView{CU: cu, Runtime: runtime}, nil
}
