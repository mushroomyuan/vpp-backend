package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetAsset struct {
	TenantID string
	ID       string
}

type GetAssetHandler decorator.QueryHandler[GetAsset, *AssetView]

type getAssetHandler struct {
	assetRepo port.AssetRepository
}

func NewGetAssetHandler(
	assetRepo port.AssetRepository,
	metricClient decorator.MetricsClient,
) GetAssetHandler {
	if assetRepo == nil {
		panic("NewGetAssetHandler parameter assetRepo is nil")
	}
	return decorator.ApplyQueryDecorators[GetAsset, *AssetView](
		getAssetHandler{assetRepo: assetRepo},
		metricClient,
	)
}

func (h getAssetHandler) Handle(ctx context.Context, q GetAsset) (*AssetView, error) {
	asset, err := h.assetRepo.FindByID(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	return &AssetView{Asset: asset}, nil
}
