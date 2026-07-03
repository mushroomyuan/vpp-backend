package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// DisableMappingByCUCode disables the DeviceMapping that is bound to the given
// CUCode (= resource CU UUID by system convention). It is the Kafka-driven
// counterpart of DisableMapping: where DisableMapping requires a mapping ID
// known to the caller, this command performs the CUCode → mapping ID lookup
// internally.
//
// If no mapping exists for the given CUCode the command is a no-op (the deleted
// resource was not a CU, or was never mapped). This keeps the consumer
// idempotent and safe to retry.
type DisableMappingByCUCode struct {
	TenantID string
	// CUCode equals the resource CU UUID (system convention: CUCode = Resource CU ID).
	CUCode string
}

type DisableMappingByCUCodeHandler = decorator.CommandHandler[DisableMappingByCUCode, struct{}]

type disableMappingByCUCodeHandler struct {
	repo port.MappingRepository
}

func NewDisableMappingByCUCodeHandler(
	repo port.MappingRepository,
	metricsClient decorator.MetricsClient,
) DisableMappingByCUCodeHandler {
	if repo == nil {
		panic("NewDisableMappingByCUCodeHandler: repo is required")
	}
	return decorator.ApplyCommandDecorators[DisableMappingByCUCode, struct{}](
		disableMappingByCUCodeHandler{repo: repo},
		metricsClient,
	)
}

func (h disableMappingByCUCodeHandler) Handle(ctx context.Context, cmd DisableMappingByCUCode) (struct{}, error) {
	tenantID := strings.TrimSpace(cmd.TenantID)
	cuCode := strings.TrimSpace(cmd.CUCode)
	if tenantID == "" || cuCode == "" {
		return struct{}{}, fmt.Errorf("tenant_id and cu_code are required")
	}

	mapping, err := h.repo.GetByCUCode(ctx, tenantID, cuCode)
	if err != nil {
		if errors.Is(err, domain.ErrMappingNotFound) {
			// No mapping for this CUCode — the deleted/disabled resource was not a
			// mapped CU. This is expected and not an error.
			return struct{}{}, nil
		}
		return struct{}{}, fmt.Errorf("lookup mapping by cu_code: %w", err)
	}

	if err := h.repo.Disable(ctx, tenantID, mapping.ID); err != nil {
		return struct{}{}, fmt.Errorf("disable mapping %s: %w", mapping.ID, err)
	}
	return struct{}{}, nil
}
