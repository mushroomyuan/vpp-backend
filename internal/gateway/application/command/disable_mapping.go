package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// DisableMapping sets a DeviceMapping to status=disabled without deleting it.
// This is the v1 mechanism for handling stale mappings when a CU is removed
// from the resource service but no Kafka lifecycle event has been published.
type DisableMapping struct {
	TenantID string
	ID       string
}

type DisableMappingResult struct{}

type DisableMappingHandler = decorator.CommandHandler[DisableMapping, *DisableMappingResult]

type disableMappingHandler struct {
	repo    port.MappingRepository
	metrics decorator.MetricsClient
}

func NewDisableMappingHandler(
	repo port.MappingRepository,
	metricsClient decorator.MetricsClient,
) DisableMappingHandler {
	if repo == nil {
		panic("NewDisableMappingHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[DisableMapping, *DisableMappingResult](
		disableMappingHandler{repo: repo, metrics: metricsClient},
		metricsClient,
	)
}

func (h disableMappingHandler) Handle(ctx context.Context, cmd DisableMapping) (*DisableMappingResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" || strings.TrimSpace(cmd.ID) == "" {
		return nil, fmt.Errorf("tenant_id and id are required")
	}
	if err := h.repo.Disable(ctx, cmd.TenantID, cmd.ID); err != nil {
		return nil, fmt.Errorf("disable mapping: %w", err)
	}
	return &DisableMappingResult{}, nil
}
