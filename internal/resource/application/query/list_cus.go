package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListCUs struct {
	TenantID      string
	SiteID        string
	AssetID       string
	CapabilityIDs []string
	IDs           []string
	NameLike      string
	Offset        int
	Limit         int
}

type ListCUsResult struct {
	Items      []*model.CU
	TotalCount int64
	Offset     int
	Limit      int
}

type ListCUsHandler decorator.QueryHandler[ListCUs, *ListCUsResult]

type listCUsHandler struct {
	cuRepo port.CURepository
}

func NewListCUsHandler(
	cuRepo port.CURepository,
	metricClient decorator.MetricsClient,
) ListCUsHandler {
	if cuRepo == nil {
		panic("NewListCUsHandler parameter cuRepo is nil")
	}
	return decorator.ApplyQueryDecorators[ListCUs, *ListCUsResult](
		listCUsHandler{cuRepo: cuRepo},
		metricClient,
	)
}

func (h listCUsHandler) Handle(ctx context.Context, q ListCUs) (*ListCUsResult, error) {
	filter := port.CUFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		SiteID:        q.SiteID,
		AssetID:       q.AssetID,
		CapabilityIDs: q.CapabilityIDs,
		IDs:           q.IDs,
		NameLike:      q.NameLike,
	}

	page, err := h.cuRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListCUsResult{
		Items:      page.Items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
