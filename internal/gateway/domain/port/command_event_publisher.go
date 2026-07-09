package port

import (
	"context"
	"time"
)

// CommandCompletedEvent is the domain-facing payload published when a control
// command reaches a terminal outcome at the gateway/EMS boundary.
// Dispatch consumes it via vpp.command.events to advance the task state machine.
type CommandCompletedEvent struct {
	TenantID     string
	CommandID    string
	CUCode       string
	Success      bool
	ErrorCode    string
	ErrorMessage string
	AckAt        *time.Time
}

// CommandEventPublisher publishes command execution outcomes for Dispatch.
type CommandEventPublisher interface {
	PublishCommandCompleted(ctx context.Context, event CommandCompletedEvent) error
}
