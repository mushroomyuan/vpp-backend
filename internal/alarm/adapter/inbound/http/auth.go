package http

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/platform/authn"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

const ginPrincipalKey = "vpp_principal"

// AuthConfig controls alarm HTTP identity / authorization middleware (Path C).
type AuthConfig struct {
	// TrustProxyHeaders when true requires a valid X-Userinfo header.
	// When false, auth is fully bypassed (local direct :8087 debug).
	TrustProxyHeaders bool
}

var tenantPathRE = regexp.MustCompile(`^/api/v1/tenants/([^/]+)(?:/|$)`)

// AuthMiddleware enforces identity, path-tenant binding, and PermissionChecker
// when TrustProxyHeaders is true. checker must be non-nil in that mode.
func AuthMiddleware(
	cfg AuthConfig,
	parsePrincipal authn.PrincipalParser,
	checker authz.PermissionChecker,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !cfg.TrustProxyHeaders {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if parsePrincipal == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "principal parser not configured",
			})
			return
		}

		principal, err := parsePrincipal(c.GetHeader("X-Userinfo"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing or invalid X-Userinfo (enable APISIX OIDC or set auth.trust-proxy-headers: false for local debug)",
			})
			return
		}

		if pathTenant, ok := pathTenantID(c.Request.URL.Path); ok {
			if pathTenant != principal.TenantID {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "tenant mismatch: path tenant does not match identity TenantID",
				})
				return
			}
		}

		if checker == nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "authorization checker not configured",
			})
			return
		}

		resource := resourceOf(c.Request.URL.Path)
		action := actionOf(c.Request.Method, c.Request.URL.Path)
		if resource == "" || action == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: role cannot perform this action",
			})
			return
		}

		decision, err := checker.Allow(c.Request.Context(), principal, resource, action)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"resource": resource,
				"action":   action,
				"user":     principal.Username,
			}).Error("authz Allow failed")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: authorization error",
			})
			return
		}
		if !decision.Allowed {
			msg := "forbidden: role cannot perform this action"
			if decision.Degraded {
				msg = "forbidden: authorization unavailable or policy stale"
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
		if decision.Degraded {
			logrus.WithFields(logrus.Fields{
				"resource": resource,
				"action":   action,
				"user":     principal.Username,
				"roles":    principal.Roles,
			}).Warn("authz decision made in degraded mode")
		}

		c.Set(ginPrincipalKey, principal)
		c.Request = c.Request.WithContext(identity.NewContext(c.Request.Context(), principal))
		c.Next()
	}
}

// PrincipalFromGin returns the authenticated principal set by AuthMiddleware.
func PrincipalFromGin(c *gin.Context) (identity.Principal, bool) {
	v, ok := c.Get(ginPrincipalKey)
	if !ok {
		return identity.Principal{}, false
	}
	principal, ok := v.(identity.Principal)
	return principal, ok
}

func pathTenantID(path string) (string, bool) {
	m := tenantPathRE.FindStringSubmatch(path)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}
