package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

type ListMappings struct {
	TenantID string
}

type ListMappingsResult struct {
	Mappings []*model.DeviceMapping
}

type ListMappingsHandler = decorator.QueryHandler[ListMappings, *ListMappingsResult]

type listMappingsHandler struct {
	repo    port.MappingRepository
	metrics decorator.MetricsClient
}

func NewListMappingsHandler(
	repo port.MappingRepository,
	metricsClient decorator.MetricsClient,
) ListMappingsHandler {
	if repo == nil {
		panic("NewListMappingsHandler: repo is required")
	}
	return decorator.ApplyQueryDecorators[ListMappings, *ListMappingsResult](
		listMappingsHandler{repo: repo, metrics: metricsClient},
		metricsClient,
	)
}

func (h listMappingsHandler) Handle(ctx context.Context, q ListMappings) (*ListMappingsResult, error) {
	if strings.TrimSpace(q.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	mappings, err := h.repo.List(ctx, q.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list mappings: %w", err)
	}
	return &ListMappingsResult{Mappings: mappings}, nil
}
