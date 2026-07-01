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

// ExecuteCommand is triggered by the dispatch service (via gRPC in production,
// or via HTTP for testing). It translates an internal CUCode to an external
// device identifier and forwards the control command to the EMSClient.
type ExecuteCommand struct {
	TenantID string
	CUCode   string
	Command  string
	Value    float64
}

// ExecuteCommandResult carries the external device ID that was actually targeted,
// useful for audit logging by the caller.
type ExecuteCommandResult struct {
	ExternalSystem string
	ExternalID     string
}

type ExecuteCommandHandler = decorator.CommandHandler[ExecuteCommand, *ExecuteCommandResult]

type executeCommandHandler struct {
	mappingRepo port.MappingRepository
	emsClient   port.EMSClient
	metrics     decorator.MetricsClient
}

func NewExecuteCommandHandler(
	mappingRepo port.MappingRepository,
	emsClient port.EMSClient,
	metricsClient decorator.MetricsClient,
) ExecuteCommandHandler {
	if mappingRepo == nil {
		panic("NewExecuteCommandHandler: mappingRepo is required")
	}
	if emsClient == nil {
		panic("NewExecuteCommandHandler: emsClient is required")
	}
	return decorator.ApplyCommandDecorators[ExecuteCommand, *ExecuteCommandResult](
		executeCommandHandler{
			mappingRepo: mappingRepo,
			emsClient:   emsClient,
			metrics:     metricsClient,
		},
		metricsClient,
	)
}

func (h executeCommandHandler) Handle(ctx context.Context, cmd ExecuteCommand) (*ExecuteCommandResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(cmd.CUCode) == "" {
		return nil, fmt.Errorf("cu_code is required")
	}
	if strings.TrimSpace(cmd.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}

	mapping, err := h.mappingRepo.GetByCUCode(ctx, cmd.TenantID, cmd.CUCode)
	if err != nil {
		if errors.Is(err, domain.ErrMappingNotFound) {
			return nil, domain.ErrMappingNotFound
		}
		return nil, fmt.Errorf("lookup mapping for cu_code %s: %w", cmd.CUCode, err)
	}
	if !mapping.IsActive() {
		return nil, domain.ErrMappingDisabled
	}

	if err := h.emsClient.SendCommand(ctx, mapping.ExternalSystem, mapping.ExternalID, cmd.Command, cmd.Value); err != nil {
		return nil, fmt.Errorf("send command to ems: %w", err)
	}

	return &ExecuteCommandResult{
		ExternalSystem: mapping.ExternalSystem,
		ExternalID:     mapping.ExternalID,
	}, nil
}
