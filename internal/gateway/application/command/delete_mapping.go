package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

type DeleteMapping struct {
	TenantID string
	ID       string
}

type DeleteMappingResult struct{}

type DeleteMappingHandler = decorator.CommandHandler[DeleteMapping, *DeleteMappingResult]

type deleteMappingHandler struct {
	repo    port.MappingRepository
	metrics decorator.MetricsClient
}

func NewDeleteMappingHandler(
	repo port.MappingRepository,
	metricsClient decorator.MetricsClient,
) DeleteMappingHandler {
	if repo == nil {
		panic("NewDeleteMappingHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[DeleteMapping, *DeleteMappingResult](
		deleteMappingHandler{repo: repo, metrics: metricsClient},
		metricsClient,
	)
}

func (h deleteMappingHandler) Handle(ctx context.Context, cmd DeleteMapping) (*DeleteMappingResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" || strings.TrimSpace(cmd.ID) == "" {
		return nil, fmt.Errorf("tenant_id and id are required")
	}
	if err := h.repo.Delete(ctx, cmd.TenantID, cmd.ID); err != nil {
		return nil, fmt.Errorf("delete mapping: %w", err)
	}
	return &DeleteMappingResult{}, nil
}
