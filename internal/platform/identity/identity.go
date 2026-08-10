// Package identity defines the transport- and identity-provider-independent
// caller identity shared by authentication and authorization components.
package identity

import "context"

type contextKey struct{}

// Principal is the authenticated VPP caller presented to authorization logic.
// It contains normalized application claims and must not expose IdP wire names.
type Principal struct {
	UserID   string
	TenantID string
	Username string
	Roles    []string
	IsAdmin  bool
}

// HasRole reports whether the principal has the given role name.
func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HighestRole returns admin > operator > viewer > "" for logging.
func (p Principal) HighestRole() string {
	switch {
	case p.HasRole("admin"):
		return "admin"
	case p.HasRole("operator"):
		return "operator"
	case p.HasRole("viewer"):
		return "viewer"
	default:
		return ""
	}
}

// NewContext returns a child context carrying the authenticated principal.
func NewContext(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

// FromContext returns the authenticated principal if present.
func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}
