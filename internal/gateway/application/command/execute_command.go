package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/sirupsen/logrus"
)

// ExecuteCommand is triggered by the dispatch service (via gRPC in production,
// or via HTTP for testing). It translates an internal CUCode to an external
// device identifier and forwards the control command to the EMSClient.
type ExecuteCommand struct {
	CommandID string
	TenantID  string
	CUCode    string
	PointKey  string
	// Value is the numeric setpoint forwarded to EMS (v1 float-only adapter).
	// Bool maps to 0/1; string values are rejected before reaching this handler.
	Value float64
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
	publisher   port.CommandEventPublisher
	metrics     decorator.MetricsClient
}

func NewExecuteCommandHandler(
	mappingRepo port.MappingRepository,
	emsClient port.EMSClient,
	publisher port.CommandEventPublisher,
	metricsClient decorator.MetricsClient,
) ExecuteCommandHandler {
	if mappingRepo == nil {
		panic("NewExecuteCommandHandler: mappingRepo is required")
	}
	if emsClient == nil {
		panic("NewExecuteCommandHandler: emsClient is required")
	}
	if publisher == nil {
		panic("NewExecuteCommandHandler: publisher is required")
	}
	return decorator.ApplyCommandDecorators[ExecuteCommand, *ExecuteCommandResult](
		executeCommandHandler{
			mappingRepo: mappingRepo,
			emsClient:   emsClient,
			publisher:   publisher,
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
	if strings.TrimSpace(cmd.PointKey) == "" {
		return nil, fmt.Errorf("point_key is required")
	}
	if strings.TrimSpace(cmd.CommandID) == "" {
		return nil, fmt.Errorf("command_id is required")
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

	if err := h.emsClient.SendCommand(
		ctx, cmd.CommandID, mapping.ExternalSystem, mapping.ExternalID, cmd.PointKey, cmd.Value,
	); err != nil {
		return nil, fmt.Errorf("send command to ems: %w", err)
	}

	// v1 ems_log is synchronous: publish CommandCompleted immediately so Dispatch
	// can advance via Kafka. Future async EMS adapters publish when the device acks.
	ackAt := time.Now()
	if pubErr := h.publisher.PublishCommandCompleted(ctx, port.CommandCompletedEvent{
		TenantID:  cmd.TenantID,
		CommandID: cmd.CommandID,
		CUCode:    cmd.CUCode,
		Success:   true,
		AckAt:     &ackAt,
	}); pubErr != nil {
		// Best-effort: do not fail the gRPC acceptance path if Kafka publish fails.
		// Dispatch TimeoutScanner / retry covers missing callbacks.
		logrus.WithError(pubErr).WithFields(logrus.Fields{
			"command_id": cmd.CommandID,
			"tenant_id":  cmd.TenantID,
		}).Error("publish CommandCompleted failed")
	}

	return &ExecuteCommandResult{
		ExternalSystem: mapping.ExternalSystem,
		ExternalID:     mapping.ExternalID,
	}, nil
}
