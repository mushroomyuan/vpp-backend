package command

import (
	"context"
	"strings"

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

	var desc *string
	if trimmed := strings.TrimSpace(cmd.Description); trimmed != "" {
		desc = &trimmed
	}
	var loc *model.Location
	if cmd.Location != (model.Location{}) {
		l := cmd.Location
		loc = &l
	}

	site, err := model.NewSite(model.CreateSiteParams{
		ID:              id,
		TenantID:        cmd.TenantID,
		ParentID:        nil,
		DisplayName:     strings.TrimSpace(cmd.Name),
		Description:     desc,
		Location:        loc,
		OperatingStatus: model.OperatingStatusUnderConstruction,
		SubType:         nil,
	})
	if err != nil {
		return nil, err
	}

	if _, err := c.siteRepo.Create(ctx, site); err != nil {
		return nil, err
	}

	return &CreateSiteResult{SiteID: id}, nil
}
