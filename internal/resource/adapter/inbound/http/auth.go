package ports

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/middleware"
)

const ginIdentityKey = "vpp_identity"

// AuthConfig controls Resource HTTP identity / RBAC middleware.
type AuthConfig struct {
	// TrustProxyHeaders when true requires a valid X-Userinfo header (APISIX Path C).
	// When false, auth is fully bypassed (local direct :8082 debug).
	TrustProxyHeaders bool
}

var tenantPathRE = regexp.MustCompile(`^/api/tenants/([^/]+)(?:/|$)`)

// AuthMiddleware enforces identity, path-tenant binding, and role RBAC when TrustProxyHeaders is true.
func AuthMiddleware(cfg AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.TrustProxyHeaders {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		id, err := middleware.ParseXUserinfo(c.GetHeader("X-Userinfo"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or invalid X-Userinfo (enable APISIX OIDC or set auth.trust-proxy-headers: false for local debug)",
			})
			return
		}

		if pathTenant, ok := pathTenantID(c.Request.URL.Path); ok {
			if pathTenant != id.TenantID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "tenant mismatch: path tenant does not match identity TenantID",
				})
				return
			}
		}

		if !allowRBAC(id, c.Request.Method, c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: role cannot perform this action",
			})
			return
		}

		c.Set(ginIdentityKey, id)
		c.Request = c.Request.WithContext(middleware.ContextWithIdentity(c.Request.Context(), id))
		c.Next()
	}
}

// IdentityFromGin returns identity set by AuthMiddleware.
func IdentityFromGin(c *gin.Context) (middleware.Identity, bool) {
	v, ok := c.Get(ginIdentityKey)
	if !ok {
		return middleware.Identity{}, false
	}
	id, ok := v.(middleware.Identity)
	return id, ok
}

func pathTenantID(path string) (string, bool) {
	m := tenantPathRE.FindStringSubmatch(path)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// allowRBAC implements the C3 matrix:
//
//	viewer:   GET only
//	operator: GET + POST/PUT/PATCH, except delete / changeLifecycle
//	admin:    all
func allowRBAC(id middleware.Identity, method, path string) bool {
	if id.HasRole("admin") {
		return true
	}

	destructive := method == http.MethodDelete || isChangeLifecycle(path)

	switch method {
	case http.MethodGet, http.MethodHead:
		return id.HasRole("viewer") || id.HasRole("operator")
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if destructive {
			return false
		}
		return id.HasRole("operator")
	case http.MethodDelete:
		return false
	default:
		return false
	}
}

func isChangeLifecycle(path string) bool {
	return strings.Contains(path, ":changeLifecycle")
}
