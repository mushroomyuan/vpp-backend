// Package authz provides a local Casbin PDP and Casdoor policy sync (AUTHZ C6).
//
// Services keep PEP (middleware) locally; this package is the replaceable port
// for "given Identity + catalog resource/action, allow or deny?".
package authz

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/middleware"
)

// PermissionChecker is the stable authorization port used by service PEPs.
// Implementations must not be called with raw HTTP paths — callers map
// (method, path) → catalog (resource, action) first (see AUTHZ plan §7.1).
type PermissionChecker interface {
	// Allow returns whether id may perform action on resource.
	// degraded=true means the decision used stale cache, invalid-tier rules,
	// or the cold-start safety net — PEPs should log/audit accordingly.
	Allow(ctx context.Context, id middleware.Identity, resource, action string) (allowed bool, degraded bool, err error)
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
