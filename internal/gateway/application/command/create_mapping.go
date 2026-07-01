package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

type CreateMapping struct {
	TenantID       string
	ExternalSystem string
	ExternalID     string
	CUCode         string
}

type CreateMappingResult struct {
	Mapping *model.DeviceMapping
}

type CreateMappingHandler = decorator.CommandHandler[CreateMapping, *CreateMappingResult]

type createMappingHandler struct {
	repo    port.MappingRepository
	metrics decorator.MetricsClient
}

func NewCreateMappingHandler(
	repo port.MappingRepository,
	metricsClient decorator.MetricsClient,
) CreateMappingHandler {
	if repo == nil {
		panic("NewCreateMappingHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[CreateMapping, *CreateMappingResult](
		createMappingHandler{repo: repo, metrics: metricsClient},
		metricsClient,
	)
}

func (h createMappingHandler) Handle(ctx context.Context, cmd CreateMapping) (*CreateMappingResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	id := idgen.Must()
	m, err := model.NewDeviceMapping(id, cmd.TenantID, cmd.ExternalSystem, cmd.ExternalID, cmd.CUCode)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now

	if err := h.repo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("persist mapping: %w", err)
	}
	return &CreateMappingResult{Mapping: m}, nil
}
