package query

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetBreadcrumb struct {
	TenantID   string
	ResourceID string
}

type GetBreadcrumbResult struct {
	Items []*model.Node
}

type GetBreadcrumbHandler decorator.QueryHandler[GetBreadcrumb, *GetBreadcrumbResult]

type getBreadcrumbHandler struct {
	nodes port.NodeRepository
}

func NewGetBreadcrumbHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) GetBreadcrumbHandler {
	if nodes == nil {
		panic("NewGetBreadcrumbHandler parameter nodes is nil")
	}
	return decorator.ApplyQueryDecorators[GetBreadcrumb, *GetBreadcrumbResult](
		getBreadcrumbHandler{nodes: nodes},
		metricClient,
	)
}

func (h getBreadcrumbHandler) Handle(ctx context.Context, q GetBreadcrumb) (*GetBreadcrumbResult, error) {
	ctx, span := telemetry.Start(ctx, "get_breadcrumb")
	defer span.End()

	page, err := h.nodes.GetAncestors(
		ctx,
		strings.TrimSpace(q.TenantID),
		strings.TrimSpace(q.ResourceID),
	)
	if err != nil {
		return nil, err
	}
	return &GetBreadcrumbResult{Items: page.Items}, nil
}
