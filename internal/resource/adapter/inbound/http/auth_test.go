package ports

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/identity"
)

type stubChecker struct {
	allowed  bool
	degraded bool
	err      error
}

func (s stubChecker) Allow(context.Context, identity.Principal, string, string) (authz.Decision, error) {
	return authz.Decision{Allowed: s.allowed, Degraded: s.degraded}, s.err
}

func TestAuthMiddleware_BypassWhenTrustFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: false}, nil, nil))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_RequireUserinfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_NilPrincipalParser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, nil, mustHealthyChecker(t)))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_TenantMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/tenants/other/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/other/sites", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ViewerCannotWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.POST("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/default/sites", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_OperatorCannotDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.DELETE("/api/tenants/default/resources/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tenants/default/resources/x", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "operator"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_AdminOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.DELETE("/api/tenants/default/resources/x", func(c *gin.Context) {
		principal, ok := PrincipalFromGin(c)
		if !ok || !principal.HasRole("admin") {
			t.Fatalf("principal missing: %+v ok=%v", principal, ok)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/tenants/default/resources/x", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "admin"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ImportJobsNoPathTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/import-jobs/abc", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/import-jobs/abc", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_ChangeLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := mustHealthyChecker(t)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, checker))
	r.POST("/api/tenants/default/resources/r:changeLifecycle", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/tenants/default/resources/r:changeLifecycle", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "operator"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("operator code=%d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/tenants/default/resources/r:changeLifecycle", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "admin"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ColdStartSafetyNet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Empty checker: TierInvalid, no policies → admin only.
	checker, err := authz.NewCheckerWithMetrics(authz.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, checker))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer cold-start code=%d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "admin"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin cold-start code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_InvalidPolicyFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, stubChecker{allowed: false, degraded: true}))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "authorization unavailable") {
		t.Fatalf("body=%s", w.Body.String())
	}
}

func TestAuthMiddleware_NilCheckerWhenTrustTrue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, nil))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "admin"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", w.Code)
	}
}

func mustHealthyChecker(t *testing.T) *authz.Checker {
	t.Helper()
	c, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter: 1 * time.Hour,
		StaleAfter:   2 * time.Hour,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules := []authz.PolicyRule{
		{"viewer", "resource:*", "read"},
		{"operator", "resource:*", "read"},
		{"admin", "resource:*", "read"},
		{"operator", "resource:*", "write"},
		{"admin", "resource:*", "write"},
		{"admin", "resource:*", "delete"},
		{"admin", "resource:*", "change-lifecycle"},
	}
	if err := c.ReplacePolicies(rules, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return c
}

func encodeUserinfo(t *testing.T, tenant, role string) string {
	t.Helper()
	payload := map[string]any{
		"sub":   "user-" + role,
		"owner": tenant,
		"name":  role,
		"roles": []map[string]string{{"name": role, "owner": tenant}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
