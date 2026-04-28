package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CreateSite struct {
	TenantID    string
	Name        string
	Location    model.Location
	Description string
}

type CreateSiteResult struct {
	SiteID string
}

type CreateSiteHandler decorator.CommandHandler[CreateSite, *CreateSiteResult]

type createSiteHandler struct {
	siteRepo port.SiteRepository
}

func NewCreateSiteHandler(
	siteRepo port.SiteRepository,
	metricClient decorator.MetricsClient,
) CreateSiteHandler {
	if siteRepo == nil {
		panic("NewCreateSiteHandler parameter siteRepo is nil")
	}
	return decorator.ApplyCommandDecorators[CreateSite, *CreateSiteResult](
		createSiteHandler{
			siteRepo: siteRepo,
		},
		metricClient,
	)
}

func (c createSiteHandler) Handle(ctx context.Context, cmd CreateSite) (*CreateSiteResult, error) {
	ctx, span := telemetry.Start(ctx, "create_site")
	defer span.End()

	id := idgen.Must()

	site, err := model.NewSiteUnderConstruction(
		id,
		cmd.TenantID,
		cmd.Name,
		cmd.Description,
		cmd.Location,
	)
	if err != nil {
		return nil, err
	}

	if _, err := c.siteRepo.Create(ctx, site); err != nil {
		return nil, err
	}

	return &CreateSiteResult{SiteID: id}, nil
}
