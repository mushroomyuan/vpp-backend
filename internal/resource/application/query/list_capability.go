package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListCUCapabilities struct {
	TenantID      string
	CUID          string
	CapabilityIDs []string
	EnabledOnly   bool
}

type ListCUCapabilitiesHandler decorator.QueryHandler[
	ListCUCapabilities,
	[]*model.CUCapability,
]

type listCUCapabilitiesHandler struct {
	repo port.CUCapabilityRepository
}

func NewListCUCapabilitiesHandler(
	repo port.CUCapabilityRepository,
	metrics decorator.MetricsClient,
) ListCUCapabilitiesHandler {
	if repo == nil {
		panic("NewListCUCapabilitiesHandler: repo is required")
	}
	return decorator.ApplyQueryDecorators[ListCUCapabilities, []*model.CUCapability](
		listCUCapabilitiesHandler{repo: repo},
		metrics,
	)
}

func (h listCUCapabilitiesHandler) Handle(
	ctx context.Context,
	q ListCUCapabilities,
) ([]*model.CUCapability, error) {
	return h.repo.List(ctx, port.CUCapabilityFilter{
		TenantID: q.TenantID, CUID: q.CUID,
		CapabilityIDs: q.CapabilityIDs, EnabledOnly: q.EnabledOnly,
	})
}
