package query

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

const (
	defaultListChildrenLimit = 50
	maxListChildrenLimit     = 200
)

type ListChildren struct {
	TenantID string
	ParentID string
	Offset   int
	Limit    int
}

type ListChildrenResult struct {
	Items      []*model.Node
	TotalCount int64
	Offset     int
	Limit      int
}

type ListChildrenHandler decorator.QueryHandler[ListChildren, *ListChildrenResult]

type listChildrenHandler struct {
	nodes port.NodeRepository
}

func NewListChildrenHandler(
	nodes port.NodeRepository,
	metricClient decorator.MetricsClient,
) ListChildrenHandler {
	if nodes == nil {
		panic("NewListChildrenHandler parameter nodes is nil")
	}
	return decorator.ApplyQueryDecorators[ListChildren, *ListChildrenResult](
		listChildrenHandler{nodes: nodes},
		metricClient,
	)
}

func (h listChildrenHandler) Handle(ctx context.Context, q ListChildren) (*ListChildrenResult, error) {
	page, err := h.nodes.ListChildren(
		ctx,
		strings.TrimSpace(q.TenantID),
		strings.TrimSpace(q.ParentID),
	)
	if err != nil {
		return nil, err
	}

	offset, limit := normalizePagination(q.Offset, q.Limit, defaultListChildrenLimit, maxListChildrenLimit)
	items := paginateNodes(page.Items, offset, limit)
	return &ListChildrenResult{
		Items:      items,
		TotalCount: int64(len(page.Items)),
		Offset:     offset,
		Limit:      limit,
	}, nil
}

func normalizePagination(offset, limit, defaultLimit, maxLimit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return offset, limit
}

func paginateNodes(items []*model.Node, offset, limit int) []*model.Node {
	if offset >= len(items) {
		return []*model.Node{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
