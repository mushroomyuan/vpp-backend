package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListSites struct {
	TenantID        string
	IDs             []string
	Status          []string
	OperatingStatus []model.OperatingStatus
	NameLike        string
	Offset          int
	Limit           int
}

type ListSitesResult struct {
	Sites      []*model.Site
	TotalCount int64
	Offset     int
	Limit      int
}

type ListSitesHandler decorator.QueryHandler[ListSites, *ListSitesResult]

type listSitesHandler struct {
	siteRepo port.SiteRepository
}

func NewListSitesHandler(
	siteRepo port.SiteRepository,
	metricClient decorator.MetricsClient,
) ListSitesHandler {
	if siteRepo == nil {
		panic("NewListSitesHandler parameter siteRepo is nil")
	}
	return decorator.ApplyQueryDecorators[ListSites, *ListSitesResult](
		listSitesHandler{siteRepo: siteRepo},
		metricClient,
	)
}

func (h listSitesHandler) Handle(ctx context.Context, q ListSites) (*ListSitesResult, error) {
	ctx, span := telemetry.Start(ctx, "list_sites")
	defer span.End()

	filter := port.SiteFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		IDs:      q.IDs,
		Status:   q.Status,
		NameLike: q.NameLike,
	}

	page, err := h.siteRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListSitesResult{
		Sites:      page.Items,
		TotalCount: page.TotalCount,
		Offset:     page.Offset,
		Limit:      page.Limit,
	}, nil
}
