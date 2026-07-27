package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ControlCommand represents a single atomic device control instruction.
//
// Its ID (UUID v7, assigned by the Application layer via platform/idgen) is the
// correlation key used for:
//   - Gateway-side idempotency (duplicate protection)
//   - Kafka event callbacks (CommandCompletedPayload.CommandID)
//   - Full-chain tracing (OpenTelemetry baggage)
//
// Protocol details (register address, external system, external device ID) are
// intentionally absent; those live in the Gateway service's DeviceMapping.
// Dispatch only knows CUCode and PointKey.
type ControlCommand struct {
	ID       string // UUID v7, globally unique
	ActionID string
	TenantID string

	CUCode   string
	PointKey string // maps to gateway proto ExecuteCommandRequest.point_key

	Value CommandValue

	Status     CommandStatus
	RetryCount int
	MaxRetries int           // max number of timeout retries; default 3
	Timeout    time.Duration // per-attempt timeout; default 30s

	SentAt     *time.Time
	DeadlineAt *time.Time // = SentAt + Timeout; indexed by TimeoutScanner
	FinishedAt *time.Time

	Result *CommandResult
}

// MarkSending transitions the command from Pending to Sending.
// It records the send time and computes the deadline for the TimeoutScanner.
func (c *ControlCommand) MarkSending(sentAt time.Time) error {
	if c.Status != CommandStatusPending {
		return fmt.Errorf("dispatch: command %s cannot transition to Sending from %q", c.ID, c.Status)
	}
	deadline := sentAt.Add(c.Timeout)
	c.Status = CommandStatusSending
	c.SentAt = &sentAt
	c.DeadlineAt = &deadline
	return nil
}

// MarkSucceeded transitions the command from Sending to Succeeded.
func (c *ControlCommand) MarkSucceeded(result *CommandResult) error {
	if c.Status != CommandStatusSending {
		return fmt.Errorf("dispatch: command %s cannot succeed from %q", c.ID, c.Status)
	}
	now := time.Now()
	c.Status = CommandStatusSucceeded
	c.FinishedAt = &now
	c.Result = result
	return nil
}

// MarkFailed transitions the command to Failed.
// Called both on explicit Gateway rejection and after exhausting retries
// (Timeout → Failed via OnCommandTimeout).
func (c *ControlCommand) MarkFailed(result *CommandResult) error {
	if c.Status != CommandStatusSending &&
		c.Status != CommandStatusPending &&
		c.Status != CommandStatusTimeout {
		return fmt.Errorf("dispatch: command %s cannot fail from %q", c.ID, c.Status)
	}
	now := time.Now()
	c.Status = CommandStatusFailed
	c.FinishedAt = &now
	c.Result = result
	return nil
}

// MarkTimeout transitions a Sending command to Timeout.
// The caller should subsequently call ResetForRetry or MarkFailed based on CanRetry.
func (c *ControlCommand) MarkTimeout() error {
	if c.Status != CommandStatusSending {
		return fmt.Errorf("dispatch: command %s cannot timeout from %q", c.ID, c.Status)
	}
	c.Status = CommandStatusTimeout
	return nil
}

// ResetForRetry transitions a Timeout command back to Pending for re-dispatch.
// Increments RetryCount and clears send-time fields. Call CanRetry first.
func (c *ControlCommand) ResetForRetry() error {
	if c.Status != CommandStatusTimeout {
		return fmt.Errorf("dispatch: command %s cannot be retried from %q", c.ID, c.Status)
	}
	c.RetryCount++
	c.Status = CommandStatusPending
	c.SentAt = nil
	c.DeadlineAt = nil
	return nil
}

// Cancel transitions a Pending command to Cancelled.
// Used exclusively by the circuit-breaker path in Dispatcher.
func (c *ControlCommand) Cancel() error {
	if c.Status != CommandStatusPending {
		return fmt.Errorf("dispatch: command %s cannot be cancelled from %q", c.ID, c.Status)
	}
	c.Status = CommandStatusCancelled
	return nil
}

// IsExpired reports whether the command has exceeded its deadline.
func (c *ControlCommand) IsExpired(now time.Time) bool {
	return c.DeadlineAt != nil && c.DeadlineAt.Before(now)
}

// CanRetry reports whether the command has remaining retry attempts after a timeout.
func (c *ControlCommand) CanRetry() bool {
	return c.RetryCount < c.MaxRetries
}

// IsTerminal reports whether the command is in a final state (no further transitions possible).
func (c *ControlCommand) IsTerminal() bool {
	switch c.Status {
	case CommandStatusSucceeded, CommandStatusFailed, CommandStatusCancelled:
		return true
	}
	return false
}

// Validate returns an error if the command is structurally invalid or missing required fields.
func (c *ControlCommand) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return errors.New("dispatch: command id is required")
	}
	if strings.TrimSpace(c.TenantID) == "" {
		return errors.New("dispatch: command tenant_id is required")
	}
	if strings.TrimSpace(c.CUCode) == "" {
		return errors.New("dispatch: command cu_code is required")
	}
	if strings.TrimSpace(c.PointKey) == "" {
		return errors.New("dispatch: command point_key is required")
	}
	if err := c.Value.Validate(); err != nil {
		return fmt.Errorf("dispatch: invalid command value: %w", err)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("dispatch: max_retries must be >= 0, got %d", c.MaxRetries)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("dispatch: timeout must be > 0, got %s", c.Timeout)
	}
	return nil
}
