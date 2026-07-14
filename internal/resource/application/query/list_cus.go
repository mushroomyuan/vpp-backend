package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListCUs struct {
	TenantID       string
	SiteID         string
	AssetID        string
	CapabilityTags []string
	IDs            []string
	NameLike       string
	Offset         int
	Limit          int
}

type ListCUsResult struct {
	Items      []*CUView
	TotalCount int64
	Offset     int
	Limit      int
}

type ListCUsHandler decorator.QueryHandler[ListCUs, *ListCUsResult]

type listCUsHandler struct {
	cuRepo    port.CURepository
	cuRuntime port.CURuntimeReader
}

func NewListCUsHandler(
	cuRepo port.CURepository,
	cuRuntime port.CURuntimeReader,
	metricClient decorator.MetricsClient,
) ListCUsHandler {
	if cuRepo == nil {
		panic("NewListCUsHandler parameter cuRepo is nil")
	}
	if cuRuntime == nil {
		panic("NewListCUsHandler parameter cuRuntime is nil")
	}
	return decorator.ApplyQueryDecorators[ListCUs, *ListCUsResult](
		listCUsHandler{cuRepo: cuRepo, cuRuntime: cuRuntime},
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
		SiteID:         q.SiteID,
		AssetID:        q.AssetID,
		CapabilityTags: q.CapabilityTags,
		IDs:            q.IDs,
		NameLike:       q.NameLike,
	}

	page, err := h.cuRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*CUView, 0, len(page.Items))
	if len(page.Items) == 0 {
		return &ListCUsResult{
			Items:      items,
			TotalCount: page.TotalCount,
			Offset:     page.Offset,
			Limit:      page.Limit,
		}, nil
	}

	cuIDs := make([]string, len(page.Items))
	for i, cu := range page.Items {
		cuIDs[i] = cu.ID
	}
	runtimes, err := h.cuRuntime.ListCURuntimes(ctx, q.TenantID, cuIDs)
	if err != nil {
		return nil, err
	}

	for i, cu := range page.Items {
		item := &CUView{CU: cu}
		if i < len(runtimes) {
			item.Runtime = runtimes[i]
		}
		items = append(items, item)
	}

	return &ListCUsResult{
		Items:      items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
