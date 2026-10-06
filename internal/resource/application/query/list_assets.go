package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
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
	Items      []*model.Asset
	TotalCount int64
	Offset     int
	Limit      int
}

type ListAssetsHandler decorator.QueryHandler[ListAssets, *ListAssetsResult]

type listAssetsHandler struct {
	assetRepo port.AssetRepository
}

func NewListAssetsHandler(
	assetRepo port.AssetRepository,
	metricClient decorator.MetricsClient,
) ListAssetsHandler {
	if assetRepo == nil {
		panic("NewListAssetsHandler parameter assetRepo is nil")
	}
	return decorator.ApplyQueryDecorators[ListAssets, *ListAssetsResult](
		listAssetsHandler{assetRepo: assetRepo},
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
	return &ListAssetsResult{
		Items:      page.Items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
