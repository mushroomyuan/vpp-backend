package policy

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CooldownKey suppresses repeat triggers of one policy direction.
// Asset and site policies share one key; CUs under that policy do not time separately.
type CooldownKey struct {
	TenantID  string
	PolicyID  string
	Direction Direction
}

// Validate checks the identity of a cooldown record.
func (k CooldownKey) Validate() error {
	if strings.TrimSpace(k.TenantID) == "" {
		return fmt.Errorf("policy: tenant_id is required")
	}
	if strings.TrimSpace(k.PolicyID) == "" {
		return fmt.Errorf("policy: policy_id is required")
	}
	return k.Direction.Validate()
}

// CooldownStore records whether a policy direction may fire.
// CoolingDown is evaluated at now. MarkTriggered stores the exclusive end of the window.
// The in-memory store is for unit tests. Postgres upsert is what the service runs,
// and the plan transaction uses it to compete for the trigger.
type CooldownStore interface {
	CoolingDown(ctx context.Context, key CooldownKey, now time.Time) (bool, error)
	MarkTriggered(ctx context.Context, key CooldownKey, until time.Time) error
}
