package model

import "time"

// CommandResult holds the final outcome of a ControlCommand execution.
// It is populated when the command reaches Succeeded or Failed state,
// either synchronously (GatewayCompleted) or via Kafka callback.
type CommandResult struct {
	Success      bool
	ErrorCode    string
	ErrorMessage string
	AckAt        *time.Time
}

// NewSuccessResult creates a CommandResult representing a successful execution.
func NewSuccessResult(ackAt time.Time) *CommandResult {
	return &CommandResult{
		Success: true,
		AckAt:   &ackAt,
	}
}

// NewFailureResult creates a CommandResult representing a failed execution.
func NewFailureResult(errorCode, errorMessage string) *CommandResult {
	return &CommandResult{
		Success:      false,
		ErrorCode:    errorCode,
		ErrorMessage: errorMessage,
	}
}
