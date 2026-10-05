package plan

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
)

// SaveInput is written in one transaction.
// A rejected plan is stored for audit. It does not take the cooldown or an outbox row.
// A ready plan claims the cooldown inside that transaction; a lost claim rolls the plan back.
type SaveInput struct {
	Objective     *objective.PowerObjective
	Plan          *Plan
	Direction     policy.Direction
	Now           time.Time
	CooldownUntil time.Time
}

// Repository stores plans and leases the outbox.
// An unsaved plan has no outbox row, so the execution loop cannot run it.
type Repository interface {
	Save(ctx context.Context, in SaveInput) error
	ClaimDue(ctx context.Context, workerID string, now time.Time, lease time.Duration) (Claim, error)
	MarkSubmitted(ctx context.Context, claim Claim, taskID string, now time.Time) error
	MarkUncertain(ctx context.Context, claim Claim, cause string, now time.Time, retryAt time.Time) error
	MarkStale(ctx context.Context, claim Claim, cause string, now time.Time) error
	MarkFailed(ctx context.Context, claim Claim, cause string, now time.Time) error
}
