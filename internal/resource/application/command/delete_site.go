package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type DeleteSite struct {
	TenantID string
	ID       string
}

type DeleteSiteHandler decorator.CommandHandler[DeleteSite, struct{}]

type deleteSiteHandler struct {
	siteRepo port.SiteRepository
}

func NewDeleteSiteHandler(
	siteRepo port.SiteRepository,
	metricClient decorator.MetricsClient,
) DeleteSiteHandler {
	if siteRepo == nil {
		panic("NewDeleteSiteHandler parameter siteRepo is nil")
	}
	return decorator.ApplyCommandDecorators[DeleteSite, struct{}](
		deleteSiteHandler{siteRepo: siteRepo},
		metricClient,
	)
}

func (h deleteSiteHandler) Handle(ctx context.Context, cmd DeleteSite) (struct{}, error) {
	ctx, span := telemetry.Start(ctx, "delete_site")
	defer span.End()

	return struct{}{}, h.siteRepo.SoftDelete(ctx, cmd.TenantID, cmd.ID)
}
