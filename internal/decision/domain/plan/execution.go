package plan

import (
	"errors"
	"strings"

	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
)

const (
	// ExecutionClaimed is an attempt a worker currently owns.
	ExecutionClaimed = "claimed"
	// ExecutionSubmitted means Dispatch accepted the task.
	ExecutionSubmitted = "submitted"
	// ExecutionUncertain means the Dispatch result was lost or timed out.
	// The next attempt reuses the same idempotency key.
	ExecutionUncertain = "uncertain"
	// ExecutionFailed is a definite Dispatch rejection. It is not retried.
	ExecutionFailed = "failed"
	// ExecutionStale means the catalog revision changed before Dispatch.
	ExecutionStale = "stale"
)

var (
	// ErrCooldownHeld means another writer already owns this policy direction.
	ErrCooldownHeld = errors.New("plan: cooldown already held")
	// ErrNoneDue means no outbox row is waiting and free of a live lease.
	ErrNoneDue = errors.New("plan: no execution due")
	// ErrLeaseLost means the worker no longer owns the claim.
	ErrLeaseLost = errors.New("plan: execution lease lost")
)

// StepIdempotencyKey is the Dispatch key for one plan step.
// Attempt is recorded on the execution row and is not part of the key, so a
// timeout retry asks Dispatch for the same task instead of creating another.
func StepIdempotencyKey(stepID string) string {
	return "plan-step:" + strings.TrimSpace(stepID)
}

// Claim is one leased plan step. Later writes are ignored unless the token still matches.
type Claim struct {
	WorkerID         string
	OutboxID         string
	LeaseToken       int64
	ExecutionID      string
	Attempt          int
	IdempotencyKey   string
	TenantID         string
	PlanID           string
	StepID           string
	StepVersion      int64
	PolicyID         string
	Scope            policy.TargetScope
	ResourceRevision string
	Commands         []PlannedCommand
}
