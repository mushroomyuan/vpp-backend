package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// GatewayAcceptanceStatus describes the synchronous outcome of a Gateway
// ExecuteCommand call. It is an integration concern and must not leak into the
// domain layer.
type GatewayAcceptanceStatus int

const (
	// GatewayAccepted means Gateway received the command; the final result will
	// arrive asynchronously via Kafka (vpp.command.events).
	GatewayAccepted GatewayAcceptanceStatus = iota

	// GatewayCompleted means Gateway finished synchronously and the result is known.
	GatewayCompleted

	// GatewayRejected means Gateway refused the command (missing mapping, disabled
	// mapping, invalid parameters, etc.).
	GatewayRejected
)

// GatewayExecuteResult is the synchronous response from GatewayPort.ExecuteCommand.
type GatewayExecuteResult struct {
	Status  GatewayAcceptanceStatus
	Success bool   // meaningful only when Status == GatewayCompleted
	Message string // optional diagnostic text (rejection reason, etc.)
}

// GatewayPort is the application-layer port for dispatching a ControlCommand
// to the Gateway service.
type GatewayPort interface {
	ExecuteCommand(ctx context.Context, cmd *model.ControlCommand) (*GatewayExecuteResult, error)
}
