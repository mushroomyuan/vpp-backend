package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetAsset struct {
	TenantID string
	ID       string
}

type GetAssetHandler decorator.QueryHandler[GetAsset, *AssetView]

type getAssetHandler struct {
	assetRepo    port.AssetRepository
	assetRuntime port.AssetRuntimeReader
}

func NewGetAssetHandler(
	assetRepo port.AssetRepository,
	assetRuntime port.AssetRuntimeReader,
	metricClient decorator.MetricsClient,
) GetAssetHandler {
	if assetRepo == nil {
		panic("NewGetAssetHandler parameter assetRepo is nil")
	}
	if assetRuntime == nil {
		panic("NewGetAssetHandler parameter assetRuntime is nil")
	}
	return decorator.ApplyQueryDecorators[GetAsset, *AssetView](
		getAssetHandler{assetRepo: assetRepo, assetRuntime: assetRuntime},
		metricClient,
	)
}

func (h getAssetHandler) Handle(ctx context.Context, q GetAsset) (*AssetView, error) {
	ctx, span := telemetry.Start(ctx, "get_asset")
	defer span.End()

	asset, err := h.assetRepo.FindByID(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	runtime, err := h.assetRuntime.GetAssetRuntime(ctx, q.TenantID, q.ID)
	if err != nil {
		return nil, err
	}
	return &AssetView{Asset: asset, Runtime: runtime}, nil
}
