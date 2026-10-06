package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mushroomyuan/vpp-backend/gateway/domain"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/binding"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/sirupsen/logrus"
)

// ExecuteCommand is triggered by the dispatch service (via gRPC in production,
// or via HTTP for testing). It translates an internal CUCode to an external
// device identifier, inverse-converts the canonical setpoint, and forwards
// the device address and raw value to the EMSClient.
type ExecuteCommand struct {
	CommandID string
	TenantID  string
	CUCode    string
	// PointKey is the canonical MetricID. Dispatch still uses this wire name.
	PointKey string
	// Value is the canonical setpoint. Bool maps to 0/1 before this handler.
	// String values are rejected before reaching this handler.
	Value float64
	// BindingRevision is the revision the caller planned against.
	// Zero means the caller did not send one. A positive value must match.
	BindingRevision int64
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
	bindings    PointBindings
	emsClient   port.EMSClient
	publisher   port.CommandEventPublisher
	metrics     decorator.MetricsClient
}

func NewExecuteCommandHandler(
	mappingRepo port.MappingRepository,
	bindings PointBindings,
	emsClient port.EMSClient,
	publisher port.CommandEventPublisher,
	metricsClient decorator.MetricsClient,
) ExecuteCommandHandler {
	if mappingRepo == nil {
		panic("NewExecuteCommandHandler: mappingRepo is required")
	}
	if bindings == nil {
		panic("NewExecuteCommandHandler: bindings are required")
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
			bindings:    bindings,
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

	snap, origin, err := h.bindings.Load(ctx, mapping.TenantID, mapping.CUCode)
	if err != nil {
		return nil, fmt.Errorf("load point bindings: %w", err)
	}
	setpoint, err := binding.TranslateDownlink(snap, origin, cmd.PointKey, cmd.Value, cmd.BindingRevision)
	if err != nil {
		var rejected *binding.DownlinkError
		if errors.As(err, &rejected) {
			h.countDownlink("rejected")
			logging.Warnf(ctx, logrus.Fields{
				"component":  "ExecuteCommand",
				"tenant_id":  mapping.TenantID,
				"cu_code":    mapping.CUCode,
				"metric_id":  strings.TrimSpace(cmd.PointKey),
				"command_id": cmd.CommandID,
				"reason":     string(rejected.Reason),
			}, "downlink command rejected")
			return nil, fmt.Errorf("%w: %s", domain.ErrCommandRejected, rejected.Reason)
		}
		return nil, err
	}

	if err := h.emsClient.SendCommand(
		ctx, cmd.CommandID, mapping.ExternalSystem, mapping.ExternalID, setpoint.ExternalAddress, setpoint.Raw,
	); err != nil {
		return nil, fmt.Errorf("send command to ems: %w", err)
	}
	h.countDownlink("sent")

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
		logging.Errorf(ctx, logrus.Fields{
			"command_id": cmd.CommandID,
			"tenant_id":  cmd.TenantID,
			"error":      pubErr.Error(),
		}, "publish CommandCompleted failed")
	}

	return &ExecuteCommandResult{
		ExternalSystem: mapping.ExternalSystem,
		ExternalID:     mapping.ExternalID,
	}, nil
}

func (h executeCommandHandler) countDownlink(status string) {
	if h.metrics == nil {
		return
	}
	h.metrics.Count("downlink", "command", status)
}
