package query

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetResourceDetail struct {
	TenantID   string
	ResourceID string
}

type GetResourceDetailHandler decorator.QueryHandler[GetResourceDetail, *model.Node]

type getResourceDetailHandler struct {
	nodes port.NodeRepository
}

func NewGetResourceDetailHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) GetResourceDetailHandler {
	if nodes == nil {
		panic("NewGetResourceDetailHandler parameter nodes is nil")
	}
	return decorator.ApplyQueryDecorators[GetResourceDetail, *model.Node](
		getResourceDetailHandler{nodes: nodes},
		metricClient,
	)
}

func (h getResourceDetailHandler) Handle(ctx context.Context, q GetResourceDetail) (*model.Node, error) {
	ctx, span := telemetry.Start(ctx, "get_resource_detail")
	defer span.End()

	return h.nodes.GetByID(
		ctx,
		strings.TrimSpace(q.TenantID),
		strings.TrimSpace(q.ResourceID),
	)
}
