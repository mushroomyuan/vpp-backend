package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetSite struct {
	TenantID string
	ID       string
}

type GetSiteHandler decorator.QueryHandler[GetSite, *model.Site]

type getSiteHandler struct {
	siteRepo port.SiteRepository
}

func NewGetSiteHandler(
	siteRepo port.SiteRepository,
	metricClient decorator.MetricsClient,
) GetSiteHandler {
	if siteRepo == nil {
		panic("NewGetSiteHandler parameter siteRepo is nil")
	}
	return decorator.ApplyQueryDecorators[GetSite, *model.Site](
		getSiteHandler{siteRepo: siteRepo},
		metricClient,
	)
}

func (h getSiteHandler) Handle(ctx context.Context, q GetSite) (*model.Site, error) {
	return h.siteRepo.FindByID(ctx, q.TenantID, q.ID)
}
