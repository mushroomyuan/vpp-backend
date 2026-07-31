package middleware

import (
	"context"
)

// Context key for Identity stored by Gin middleware / tests.
type identityCtxKey struct{}

// Identity is the VPP caller-identity contract used by Resource (and future services).
//
// Downstream code must depend only on this type — never on IdP claim names (owner, roles[].name, …).
// The current ingress mapping from APISIX X-Userinfo is Casdoor-shaped; see casdoor_userinfo.go
// and docs/CASDOOR.md §7.5 / §7.6.
type Identity struct {
	UserID   string   // stable caller id
	TenantID string   // VPP tenant; path /api/tenants/{id} must match when present
	Username string   // display / login name (logging, audit)
	Roles    []string // VPP role names: admin | operator | viewer
	IsAdmin  bool     // optional IdP hint; RBAC uses Roles, not this flag
}

// HasRole reports whether id has the given role name.
func (id Identity) HasRole(role string) bool {
	for _, r := range id.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HighestRole returns admin > operator > viewer > "" for logging.
func (id Identity) HighestRole() string {
	switch {
	case id.HasRole("admin"):
		return "admin"
	case id.HasRole("operator"):
		return "operator"
	case id.HasRole("viewer"):
		return "viewer"
	default:
		return ""
	}
}

// ContextWithIdentity returns a child context carrying id.
func ContextWithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityCtxKey{}, id)
}

// IdentityFromContext returns identity if present.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityCtxKey{}).(Identity)
	return id, ok
}
