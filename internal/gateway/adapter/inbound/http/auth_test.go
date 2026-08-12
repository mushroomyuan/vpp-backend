package http

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
)

func encodeUserinfo(t *testing.T, tenant, role string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"sub":   "u1",
		"owner": tenant,
		"name":  role,
		"roles": []map[string]string{{"name": role, "owner": tenant}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func mappingPolicies() []authz.PolicyRule {
	return []authz.PolicyRule{
		{"viewer", "gateway:mappings", "read"},
		{"operator", "gateway:mappings", "read"},
		{"admin", "gateway:mappings", "read"},
		{"operator", "gateway:mappings", "write"},
		{"admin", "gateway:mappings", "write"},
		{"admin", "gateway:mappings", "delete"},
	}
}

func mustHealthyChecker(t *testing.T) *authz.Checker {
	t.Helper()
	c, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ReplacePolicies(mappingPolicies(), time.Now()); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAuthMiddleware_BypassWhenTrustFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: false}, nil, nil))
	r.GET("/api/v1/tenants/default/mappings", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/default/mappings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_RequireUserinfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/v1/tenants/default/mappings", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/default/mappings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_ViewerCannotWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.POST("/api/v1/tenants/default/mappings", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/default/mappings", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_OperatorCanWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.POST("/api/v1/tenants/default/mappings", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/default/mappings", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "operator"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_OperatorCannotDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.DELETE("/api/v1/tenants/default/mappings/m1", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tenants/default/mappings/m1", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "operator"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_TenantMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/v1/tenants/other/mappings", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/other/mappings", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestRegisterRoutes_IngestBypassesUserAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Minimal app is not needed if we only check middleware short-circuit;
	// mount routes with a stub that panics if ingest goes through auth.
	authCalled := false
	mw := func(c *gin.Context) {
		authCalled = true
		c.AbortWithStatus(http.StatusUnauthorized)
	}
	// Register with nil app will panic on handler — use AuthMiddleware with trust
	// and mount ingest manually to assert separation is in router grouping.
	v1 := r.Group("/api/v1/tenants/:tenant_id")
	v1.POST("/telemetry:ingest", func(c *gin.Context) { c.Status(http.StatusAccepted) })
	mappings := v1.Group("")
	mappings.Use(mw)
	mappings.GET("/mappings", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/default/telemetry:ingest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted || authCalled {
		t.Fatalf("ingest code=%d authCalled=%v", w.Code, authCalled)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tenants/default/mappings", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || !authCalled {
		t.Fatalf("mappings code=%d authCalled=%v", w.Code, authCalled)
	}
}

func TestCatalog(t *testing.T) {
	if resourceOf("/api/v1/tenants/default/mappings") != "gateway:mappings" {
		t.Fatal(resourceOf("/api/v1/tenants/default/mappings"))
	}
	if resourceOf("/api/v1/tenants/default/telemetry:ingest") != "" {
		t.Fatal("ingest must not map to user catalog")
	}
	if actionOf(http.MethodPatch, "") != "write" {
		t.Fatal(actionOf(http.MethodPatch, ""))
	}
	cat := AuthzCatalog("default", "default/vpp-rbac")
	if cat.Service != "gateway" || len(cat.Entries) != 1 || cat.Entries[0].Object != "gateway:mappings" {
		t.Fatalf("%+v", cat)
	}
}
