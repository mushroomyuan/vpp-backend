package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type ListCUs struct {
	TenantID   string
	SiteID     string
	ResourceID string
	ParentCUID string
	Capability []string
	IDs        []string
	NameLike   string
	Offset     int
	Limit      int
}

type ListCUsResult struct {
	CUs []*model.CU
}

type ListCUsHandler decorator.QueryHandler[ListCUs, *ListCUsResult]

type listCUsHandler struct {
	cuRepo port.CURepository
}

func NewListCUsHandler(
	cuRepo port.CURepository,
	metricClient decorator.MetricsClient,
) ListCUsHandler {
	if cuRepo == nil {
		panic("NewListCUsHandler parameter cuRepo is nil")
	}
	return decorator.ApplyQueryDecorators[ListCUs, *ListCUsResult](
		listCUsHandler{cuRepo: cuRepo},
		metricClient,
	)
}

func (h listCUsHandler) Handle(ctx context.Context, q ListCUs) (*ListCUsResult, error) {
	ctx, span := telemetry.Start(ctx, "list_cus")
	defer span.End()

	filter := port.CUFilter{
		BaseFilter: port.BaseFilter{
			TenantID: q.TenantID,
			Offset:   q.Offset,
			Limit:    q.Limit,
		},
		SiteID:     q.SiteID,
		ResourceID: q.ResourceID,
		ParentCUID: q.ParentCUID,
		Capability: q.Capability,
		IDs:        q.IDs,
		NameLike:   q.NameLike,
	}

	cus, err := h.cuRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &ListCUsResult{CUs: cus}, nil
}
