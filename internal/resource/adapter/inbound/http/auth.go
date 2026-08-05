package ports

import (
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/middleware"
	"github.com/sirupsen/logrus"
)

const ginIdentityKey = "vpp_identity"

// AuthConfig controls Resource HTTP identity / authorization middleware.
type AuthConfig struct {
	// TrustProxyHeaders when true requires a valid X-Userinfo header (APISIX Path C).
	// When false, auth is fully bypassed (local direct :8082 debug).
	TrustProxyHeaders bool
}

var tenantPathRE = regexp.MustCompile(`^/api/tenants/([^/]+)(?:/|$)`)

// AuthMiddleware enforces identity, path-tenant binding, and PermissionChecker
// when TrustProxyHeaders is true. checker must be non-nil in that mode.
func AuthMiddleware(cfg AuthConfig, checker authz.PermissionChecker) gin.HandlerFunc {
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

		allowed, degraded, err := checker.Allow(c.Request.Context(), id, resource, action)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"resource": resource,
				"action":   action,
				"user":     id.Username,
			}).Error("authz Allow failed")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden: authorization error",
			})
			return
		}
		if !allowed {
			msg := "forbidden: role cannot perform this action"
			if degraded {
				msg = "forbidden: authorization unavailable or policy stale"
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
		if degraded {
			logrus.WithFields(logrus.Fields{
				"resource": resource,
				"action":   action,
				"user":     id.Username,
				"roles":    id.Roles,
			}).Warn("authz decision made in degraded mode")
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
