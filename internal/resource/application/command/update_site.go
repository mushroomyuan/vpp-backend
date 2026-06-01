package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type UpdateSite struct {
	TenantID    string
	ID          string
	Name        string
	Location    model.Location
	Description string
}

type UpdateSiteHandler decorator.CommandHandler[UpdateSite, struct{}]

type updateSiteHandler struct {
	siteRepo port.SiteRepository
}

func NewUpdateSiteHandler(
	siteRepo port.SiteRepository,
	metricClient decorator.MetricsClient,
) UpdateSiteHandler {
	if siteRepo == nil {
		panic("NewUpdateSiteHandler parameter siteRepo is nil")
	}
	return decorator.ApplyCommandDecorators[UpdateSite, struct{}](
		updateSiteHandler{siteRepo: siteRepo},
		metricClient,
	)
}

func (h updateSiteHandler) Handle(ctx context.Context, cmd UpdateSite) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "update_site")
	defer span.End()

	site, err := h.siteRepo.FindByID(ctx, cmd.TenantID, cmd.ID)
	if err != nil {
		return struct{}{}, err
	}

	if err := site.Rename(cmd.Name); err != nil {
		return struct{}{}, err
	}
	site.UpdateDescription(cmd.Description)

	var loc *model.Location
	if cmd.Location != (model.Location{}) {
		l := cmd.Location
		loc = &l
	}
	site.SetLocation(loc)

	if err = h.siteRepo.Update(ctx, site); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}
