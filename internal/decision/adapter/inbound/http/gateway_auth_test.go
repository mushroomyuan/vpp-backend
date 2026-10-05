package ports

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
	grpcinbound "github.com/mushroomyuan/vpp-backend/decision/adapter/inbound/grpc"
	"github.com/mushroomyuan/vpp-backend/decision/adapter/outbound/memory"
	"github.com/mushroomyuan/vpp-backend/decision/application"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/middleware/grpcauth"
)

func TestDialTarget(t *testing.T) {
	if got := DialTarget(":5008"); got != "127.0.0.1:5008" {
		t.Fatalf("DialTarget = %s", got)
	}
	if got := DialTarget("10.0.0.8:5008"); got != "10.0.0.8:5008" {
		t.Fatalf("DialTarget = %s", got)
	}
}

func TestGateway_AuthAndEnablePrecheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resource := &staticResource{ok: false}
	app := application.New(application.Dependencies{
		Policies: memory.NewPolicyRepository(),
		Resource: resource,
		Metrics:  nopMetrics{},
	})
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		gatewayChecker(t),
		grpcinbound.CatalogOf,
		grpcauth.ProtoTenantID,
	)
	grpcSrv := grpc.NewServer(grpc.UnaryInterceptor(interceptor))
	decisionpb.RegisterDecisionServiceServer(grpcSrv, grpcinbound.NewServer(app))
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(func() {
		grpcSrv.Stop()
		_ = lis.Close()
	})

	engine := gin.New()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := Mount(ctx, engine, lis.Addr().String()); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{
		"Name": "asset soc",
		"Kind": "POLICY_KIND_SOC_THRESHOLD",
		"Scope": {"Type": "SCOPE_TYPE_ASSET", "ID": "asset-1"},
		"Cooldown": "120s",
		"SOC": {"MinSOC": 20, "MaxSOC": 90, "ChargePowerKW": 100, "DischargePowerKW": 80}
	}`)
	if status, _ := do(engine, http.MethodPost, "/api/tenants/default/policies", nil, body); status != http.StatusUnauthorized {
		t.Fatalf("missing userinfo status = %d", status)
	}
	if status, _ := do(engine, http.MethodPost, "/api/tenants/default/policies", userinfo(t, "other", "admin"), body); status != http.StatusForbidden {
		t.Fatalf("cross-tenant status = %d", status)
	}
	if status, _ := do(engine, http.MethodPost, "/api/tenants/default/policies", userinfo(t, "default", "viewer"), body); status != http.StatusForbidden {
		t.Fatalf("viewer write status = %d", status)
	}

	status, raw := do(engine, http.MethodPost, "/api/tenants/default/policies", userinfo(t, "default", "admin"), body)
	if status != http.StatusOK {
		t.Fatalf("create status = %d body %s", status, raw)
	}
	var created map[string]any
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["ID"].(string)
	if id == "" || created["Enabled"] == true {
		t.Fatalf("created = %s", raw)
	}

	enableBody := []byte(`{"Version": 1}`)
	status, raw = do(engine, http.MethodPost, "/api/tenants/default/policies/"+id+":enable", userinfo(t, "default", "admin"), enableBody)
	if status != http.StatusBadRequest {
		t.Fatalf("enable precheck status = %d body %s", status, raw)
	}
	if !bytes.Contains(raw, []byte("cu-1")) || !bytes.Contains(raw, []byte("missing_metric_binding")) {
		t.Fatalf("precheck body = %s", raw)
	}
}

type staticResource struct{ ok bool }

func (s *staticResource) ResolveScope(context.Context, port.ScopeQuery) (port.ResolvedScope, error) {
	if s.ok {
		return port.ResolvedScope{PrecheckOK: true, Members: []port.ResolvedCU{{CUID: "cu-1"}}}, nil
	}
	return port.ResolvedScope{
		PrecheckOK: false,
		PrecheckFailures: []port.ScopePrecheckFailure{{
			CUID:   "cu-1",
			Reason: port.PrecheckMissingMetricBinding,
			Detail: "soc read",
		}},
	}, nil
}

func do(engine *gin.Engine, method, path string, header map[string]string, body []byte) (int, []byte) {
	var lastStatus int
	var lastBody []byte
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		lastStatus = rec.Code
		lastBody, _ = io.ReadAll(rec.Body)
		if lastStatus != http.StatusServiceUnavailable {
			return lastStatus, lastBody
		}
		time.Sleep(20 * time.Millisecond)
	}
	return lastStatus, lastBody
}

func gatewayChecker(t *testing.T) *authz.Checker {
	t.Helper()
	checker, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter:        time.Minute,
		StaleAfter:          5 * time.Minute,
		DenyWritesWhenStale: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := checker.ReplacePolicies([]authz.PolicyRule{
		{"viewer", "decision:policies", "read"},
		{"admin", "decision:policies", "read"},
		{"admin", "decision:policies", "write"},
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	return checker
}

func userinfo(t *testing.T, tenant, role string) map[string]string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"sub":   "u1",
		"owner": tenant,
		"name":  role,
		"roles": []map[string]string{{"name": role, "owner": tenant}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"X-Userinfo": base64.StdEncoding.EncodeToString(raw)}
}

type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }

var _ decorator.MetricsClient = nopMetrics{}
