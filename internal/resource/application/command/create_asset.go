package command

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateAsset struct {
	TenantID        string
	SiteID          string
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

type CreateAssetResult struct {
	AssetID string
}

type CreateAssetHandler decorator.CommandHandler[CreateAsset, *CreateAssetResult]

type createAssetHandler struct {
	assetRepo port.AssetRepository
	publisher port.ResourceEventPublisher
}

func NewCreateAssetHandler(
	assetRepo port.AssetRepository,
	metricClient decorator.MetricsClient,
	publisher port.ResourceEventPublisher,
) CreateAssetHandler {
	if assetRepo == nil {
		panic("NewCreateAssetHandler parameter assetRepo is nil")
	}
	return decorator.ApplyCommandDecorators[CreateAsset, *CreateAssetResult](
		createAssetHandler{assetRepo: assetRepo, publisher: publisher},
		metricClient,
	)
}

func (h createAssetHandler) Handle(ctx context.Context, cmd CreateAsset) (*CreateAssetResult, error) {
	ctx, span := telemetry.Start(ctx, "create_asset")
	defer span.End()

	id := idgen.Must()

	siteID := strings.TrimSpace(cmd.SiteID)
	var parentID *string
	if siteID != "" {
		parentID = &siteID
	}

	dispatch := cmd.DispatchStatus
	if strings.TrimSpace(string(dispatch)) == "" {
		dispatch = model.DispatchStatusUnknown
	}

	asset, err := model.NewAsset(model.CreateAssetParams{
		ID:              id,
		TenantID:        cmd.TenantID,
		ParentID:        parentID,
		DisplayName:     strings.TrimSpace(cmd.Name),
		DispatchStatus:  dispatch,
		RatedCapacityKW: cmd.RatedCapacityKW,
		DispatchMode:    cmd.DispatchMode,
		EnergyType:      cmd.EnergyType,
		OwnerType:       cmd.OwnerType,
		SubType:         cmd.SubType,
		Description:     cmd.Description,
		MarketEnabled:   cmd.MarketEnabled,
	})
	if err != nil {
		return nil, err
	}

	if cmd.Metadata != nil {
		for k, v := range cmd.Metadata {
			asset.Metadata[k] = v
		}
	}

	if _, err := h.assetRepo.Create(ctx, asset); err != nil {
		return nil, err
	}

	if h.publisher != nil {
		if pubErr := h.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeAssetCreated,
			TenantID:   cmd.TenantID,
			ResourceID: id,
			Payload: platEvent.AssetCreatedPayload{
				AssetID:  id,
				TenantID: cmd.TenantID,
				SiteID:   siteID,
				Name:     strings.TrimSpace(cmd.Name),
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish asset created event")
		}
	}

	return &CreateAssetResult{AssetID: id}, nil
}
