package ports

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mushroomyuan/vpp-backend/platform/middleware"
)

func TestAuthMiddleware_BypassWhenTrustFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: false}))
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
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}))
	r.GET("/api/tenants/default/sites", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/tenants/default/sites", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_TenantMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}))
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
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}))
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
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}))
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
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}))
	r.DELETE("/api/tenants/default/resources/x", func(c *gin.Context) {
		id, ok := IdentityFromGin(c)
		if !ok || !id.HasRole("admin") {
			t.Fatalf("identity missing: %+v ok=%v", id, ok)
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
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}))
	r.GET("/api/import-jobs/abc", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/import-jobs/abc", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAllowRBAC_ChangeLifecycle(t *testing.T) {
	op := middleware.Identity{Roles: []string{"operator"}}
	if allowRBAC(op, http.MethodPost, "/api/tenants/default/resources/r:changeLifecycle") {
		t.Fatal("operator must not changeLifecycle")
	}
	admin := middleware.Identity{Roles: []string{"admin"}}
	if !allowRBAC(admin, http.MethodPost, "/api/tenants/default/resources/r:changeLifecycle") {
		t.Fatal("admin must changeLifecycle")
	}
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
