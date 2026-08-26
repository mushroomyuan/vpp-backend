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
		"sub":   "u-" + role,
		"owner": tenant,
		"name":  role,
		"roles": []map[string]string{{"name": role, "owner": tenant}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func alarmPolicies() []authz.PolicyRule {
	return []authz.PolicyRule{
		{"viewer", catalogObject, "read"},
		{"operator", catalogObject, "read"},
		{"operator", catalogObject, "ack"},
		{"operator", catalogObject, "close"},
		{"admin", catalogObject, "read"},
		{"admin", catalogObject, "ack"},
		{"admin", catalogObject, "close"},
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
	if err := c.ReplacePolicies(alarmPolicies(), time.Now()); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAuthMiddleware_BypassWhenTrustFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: false}, nil, nil))
	r.GET("/api/v1/tenants/default/alarms", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/default/alarms", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_RequireUserinfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/v1/tenants/default/alarms", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/default/alarms", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_ViewerCannotAck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.POST("/api/v1/tenants/default/alarms/a1/ack", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenants/default/alarms/a1/ack", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_OperatorCanAckAndClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.POST("/api/v1/tenants/default/alarms/a1/ack", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/api/v1/tenants/default/alarms/a1/close", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{
		"/api/v1/tenants/default/alarms/a1/ack",
		"/api/v1/tenants/default/alarms/a1/close",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "operator"))
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s code=%d", path, w.Code)
		}
	}
}

func TestAuthMiddleware_TenantMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/v1/tenants/other/alarms", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/other/alarms", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("code=%d", w.Code)
	}
}

func TestAuthMiddleware_ViewerCanRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(AuthConfig{TrustProxyHeaders: true}, casdoor.ParseUserinfo, mustHealthyChecker(t)))
	r.GET("/api/v1/tenants/default/alarms", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tenants/default/alarms", nil)
	req.Header.Set("X-Userinfo", encodeUserinfo(t, "default", "viewer"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
}
