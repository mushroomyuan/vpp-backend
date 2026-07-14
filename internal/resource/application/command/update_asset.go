package command

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdateAsset struct {
	ID              string
	TenantID        string
	Name            string
	DispatchStatus  model.DispatchStatus
	RatedCapacityKW *float64
	DispatchMode    *string
	EnergyType      *string
	OwnerType       *string
	SubType         *string
	Description     *string
	MarketEnabled   *bool
	Metadata        map[string]any
}

type UpdateAssetHandler decorator.CommandHandler[UpdateAsset, struct{}]

type updateAssetHandler struct {
	assetRepo port.AssetRepository
	publisher port.ResourceEventPublisher
}

func NewUpdateAssetHandler(
	assetRepo port.AssetRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) UpdateAssetHandler {
	if assetRepo == nil {
		panic("NewUpdateAssetHandler parameter assetRepo is nil")
	}
	return decorator.ApplyCommandDecorators[UpdateAsset, struct{}](
		updateAssetHandler{assetRepo: assetRepo, publisher: publisher},
		metricClient,
	)
}

func (h updateAssetHandler) Handle(ctx context.Context, cmd UpdateAsset) (struct{}, error) {
	asset, err := h.assetRepo.FindByID(ctx, cmd.TenantID, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}

	if err := asset.Rename(cmd.Name); err != nil {
		return struct{}{}, err
	}

	if ds := strings.TrimSpace(string(cmd.DispatchStatus)); ds != "" {
		asset.UpdateDispatchStatus(model.DispatchStatus(ds))
	}

	if err := asset.SetRatedCapacityKW(cmd.RatedCapacityKW); err != nil {
		return struct{}{}, err
	}

	asset.DispatchMode = normalizeOptionalStringPtr(cmd.DispatchMode)
	asset.EnergyType = normalizeOptionalStringPtr(cmd.EnergyType)
	asset.OwnerType = normalizeOptionalStringPtr(cmd.OwnerType)
	asset.SubType = normalizeOptionalStringPtr(cmd.SubType)

	if cmd.MarketEnabled != nil {
		asset.SetMarketEnabled(*cmd.MarketEnabled)
	}

	if cmd.Description != nil {
		asset.UpdateDescription(*cmd.Description)
	}

	if asset.Metadata == nil {
		asset.Metadata = make(map[string]any)
	}
	if cmd.Metadata != nil {
		for k, v := range cmd.Metadata {
			asset.Metadata[k] = v
		}
	}

	if err = h.assetRepo.Update(ctx, asset); err != nil {
		return struct{}{}, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeAssetUpdated,
			TenantID:   cmd.TenantID,
			ResourceID: cmd.ID,
			Payload: platEvent.AssetUpdatedPayload{
				AssetID:  cmd.ID,
				TenantID: cmd.TenantID,
				Name:     cmd.Name,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish asset updated event")
		}
	}

	return struct{}{}, nil
}

// normalizeOptionalStringPtr trims *string from the command layer; nil stays nil,
// empty/whitespace becomes nil (clear field).
func normalizeOptionalStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	s := strings.TrimSpace(*p)
	if s == "" {
		return nil
	}
	return &s
}
