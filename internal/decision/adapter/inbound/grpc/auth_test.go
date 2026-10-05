package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	decisionpb "github.com/mushroomyuan/vpp-backend/api/decision/proto/gen"
	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/middleware/grpcauth"
)

func TestCatalogOf(t *testing.T) {
	obj, act, ok := CatalogOf(decisionpb.DecisionService_EnablePolicy_FullMethodName)
	if !ok || obj != "decision:policies" || act != "write" {
		t.Fatalf("enable catalog = %s %s %v", obj, act, ok)
	}
	obj, act, ok = CatalogOf(decisionpb.DecisionService_ListPolicies_FullMethodName)
	if !ok || obj != "decision:policies" || act != "read" {
		t.Fatalf("list catalog = %s %s %v", obj, act, ok)
	}
	if _, _, ok := CatalogOf("/decisionpb.DecisionService/Unknown"); ok {
		t.Fatal("unknown method should be uncatalogued")
	}
}

func TestAuthzCatalog(t *testing.T) {
	cat := AuthzCatalog("default", "default/vpp-rbac")
	if cat.Service != "decision" || len(cat.Entries) != 1 || cat.Entries[0].Object != "decision:policies" {
		t.Fatalf("catalog = %+v", cat)
	}
}

func TestAuth_ViewerCannotWrite(t *testing.T) {
	err := callCreate(t, authInterceptor(t), userContext(t, "default", "viewer"), "default")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %s err %v", status.Code(err), err)
	}
}

func TestAuth_CrossTenantDenied(t *testing.T) {
	err := callCreate(t, authInterceptor(t), userContext(t, "other", "admin"), "default")
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %s err %v", status.Code(err), err)
	}
}

func TestAuth_AdminCanWriteOwnTenant(t *testing.T) {
	err := callCreate(t, authInterceptor(t), userContext(t, "default", "admin"), "default")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuth_MissingUserinfo(t *testing.T) {
	err := callCreate(t, authInterceptor(t), context.Background(), "default")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %s err %v", status.Code(err), err)
	}
}

func TestAuth_BypassWhenTrustDisabled(t *testing.T) {
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: false},
		casdoor.ParseUserinfo,
		nil,
		CatalogOf,
		grpcauth.ProtoTenantID,
	)
	if err := callCreate(t, interceptor, context.Background(), "default"); err != nil {
		t.Fatal(err)
	}
}

func authInterceptor(t *testing.T) grpc.UnaryServerInterceptor {
	t.Helper()
	return grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		mustChecker(t),
		CatalogOf,
		grpcauth.ProtoTenantID,
	)
}

func callCreate(t *testing.T, interceptor grpc.UnaryServerInterceptor, ctx context.Context, tenant string) error {
	t.Helper()
	_, err := interceptor(ctx, &decisionpb.CreatePolicyRequest{TenantID: tenant}, &grpc.UnaryServerInfo{
		FullMethod: decisionpb.DecisionService_CreatePolicy_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &decisionpb.Policy{}, nil
	})
	return err
}

func mustChecker(t *testing.T) *authz.Checker {
	t.Helper()
	checker, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter:        time.Minute,
		StaleAfter:          5 * time.Minute,
		DenyWritesWhenStale: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules := []authz.PolicyRule{
		{"viewer", "decision:policies", "read"},
		{"operator", "decision:policies", "read"},
		{"admin", "decision:policies", "read"},
		{"operator", "decision:policies", "write"},
		{"admin", "decision:policies", "write"},
	}
	if err := checker.ReplacePolicies(rules, time.Now()); err != nil {
		t.Fatal(err)
	}
	return checker
}

func userContext(t *testing.T, tenant, role string) context.Context {
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
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, base64.StdEncoding.EncodeToString(raw))
	return metadata.NewIncomingContext(context.Background(), md)
}
