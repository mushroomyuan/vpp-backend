package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListAssets struct {
	TenantID string
	SiteID   string
	IDs      []string
	Types    []string
	NameLike string
	Offset   int
	Limit    int
}

type ListAssetsResult struct {
	Items      []*AssetView
	TotalCount int64
	Offset     int
	Limit      int
}

type ListAssetsHandler decorator.QueryHandler[ListAssets, *ListAssetsResult]

type listAssetsHandler struct {
	assetRepo    port.AssetRepository
	assetRuntime port.AssetRuntimeReader
}

func NewListAssetsHandler(
	assetRepo port.AssetRepository,
	assetRuntime port.AssetRuntimeReader,
	metricClient decorator.MetricsClient,
) ListAssetsHandler {
	if assetRepo == nil {
		panic("NewListAssetsHandler parameter assetRepo is nil")
	}
	if assetRuntime == nil {
		panic("NewListAssetsHandler parameter assetRuntime is nil")
	}
	return decorator.ApplyQueryDecorators[ListAssets, *ListAssetsResult](
		listAssetsHandler{assetRepo: assetRepo, assetRuntime: assetRuntime},
		metricClient,
	)
}

func (h listAssetsHandler) Handle(ctx context.Context, q ListAssets) (*ListAssetsResult, error) {
	filter := port.AssetFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		SiteID:   q.SiteID,
		IDs:      q.IDs,
		Types:    q.Types,
		NameLike: q.NameLike,
	}

	page, err := h.assetRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]*AssetView, 0, len(page.Items))
	if len(page.Items) == 0 {
		return &ListAssetsResult{
			Items:      items,
			TotalCount: page.TotalCount,
			Offset:     page.Offset,
			Limit:      page.Limit,
		}, nil
	}

	assetIDs := make([]string, len(page.Items))
	for i, asset := range page.Items {
		assetIDs[i] = asset.ID
	}
	runtimes, err := h.assetRuntime.ListAssetRuntimes(ctx, q.TenantID, assetIDs)
	if err != nil {
		return nil, err
	}

	for i, asset := range page.Items {
		item := &AssetView{Asset: asset}
		if i < len(runtimes) {
			item.Runtime = runtimes[i]
		}
		items = append(items, item)
	}

	return &ListAssetsResult{
		Items:      items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
