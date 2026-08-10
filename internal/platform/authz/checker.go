// Package authz provides a local Casbin PDP and Casdoor policy sync (AUTHZ C6).
//
// Services keep PEP (middleware) locally; this package is the replaceable port
// for "given Principal + catalog resource/action, allow or deny?".
package authz

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

// Decision is the result of one authorization check.
// Degraded indicates that stale/invalid policy handling or the cold-start
// safety net influenced the result and should be surfaced to audit/metrics.
type Decision struct {
	Allowed  bool
	Degraded bool
}

// PermissionChecker is the stable authorization port used by service PEPs.
// Implementations must not be called with raw HTTP paths — callers map
// (method, path) → catalog (resource, action) first (see AUTHZ plan §7.1).
type PermissionChecker interface {
	// Allow reports whether principal may perform action on resource.
	Allow(ctx context.Context, principal identity.Principal, resource, action string) (Decision, error)
}

// Tier is the policy-sync health band (§6.1).
type Tier int

const (
	// TierInvalid: never synced successfully, or staleness >= StaleAfter.
	TierInvalid Tier = iota
	// TierStale: HealthyAfter <= staleness < StaleAfter — cache still used.
	TierStale
	// TierHealthy: staleness < HealthyAfter.
	TierHealthy
)

func (t Tier) String() string {
	switch t {
	case TierHealthy:
		return "healthy"
	case TierStale:
		return "stale"
	default:
		return "invalid"
	}
}
