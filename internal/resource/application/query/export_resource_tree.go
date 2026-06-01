package query

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

const (
	defaultExportMaxDepth = 3
	maxExportDepth        = 8
)

type ExportResourceTree struct {
	TenantID   string
	RootID     string
	MaxDepth   int
}

type ExportResourceTreeResult struct {
	Items []*model.Node
}

type ExportResourceTreeHandler decorator.QueryHandler[ExportResourceTree, *ExportResourceTreeResult]

type exportResourceTreeHandler struct {
	nodes port.NodeRepository
}

func NewExportResourceTreeHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) ExportResourceTreeHandler {
	if nodes == nil {
		panic("NewExportResourceTreeHandler parameter nodes is nil")
	}
	return decorator.ApplyQueryDecorators[ExportResourceTree, *ExportResourceTreeResult](
		exportResourceTreeHandler{nodes: nodes},
		metricClient,
	)
}

func (h exportResourceTreeHandler) Handle(ctx context.Context, q ExportResourceTree) (*ExportResourceTreeResult, error) {
	ctx, span := telemetry.Start(ctx, "export_resource_tree")
	defer span.End()

	tenantID := strings.TrimSpace(q.TenantID)
	rootID := strings.TrimSpace(q.RootID)
	maxDepth := q.MaxDepth
	if maxDepth <= 0 {
		maxDepth = defaultExportMaxDepth
	}
	if maxDepth > maxExportDepth {
		maxDepth = maxExportDepth
	}

	root, err := h.nodes.GetByID(ctx, tenantID, rootID)
	if err != nil {
		return nil, err
	}

	descendants, err := h.nodes.ListDescendants(ctx, tenantID, rootID)
	if err != nil {
		return nil, err
	}

	items := make([]*model.Node, 0, len(descendants.Items)+1)
	items = append(items, root)
	for _, node := range descendants.Items {
		if node.Depth-root.Depth <= maxDepth {
			items = append(items, node)
		}
	}
	return &ExportResourceTreeResult{Items: items}, nil
}
