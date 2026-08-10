package grpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/middleware/grpcauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
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

func dispatchPolicies() []authz.PolicyRule {
	return []authz.PolicyRule{
		{"viewer", "dispatch:tasks", "read"},
		{"operator", "dispatch:tasks", "read"},
		{"admin", "dispatch:tasks", "read"},
		{"operator", "dispatch:tasks", "submit"},
		{"admin", "dispatch:tasks", "submit"},
		{"admin", "dispatch:tasks", "cancel"},
	}
}

func mustHealthyChecker(t *testing.T) *authz.Checker {
	t.Helper()
	c, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter:        1 * time.Minute,
		StaleAfter:          5 * time.Minute,
		DenyWritesWhenStale: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := c.ReplacePolicies(dispatchPolicies(), now); err != nil {
		t.Fatal(err)
	}
	return c
}

func callSubmit(t *testing.T, interceptor grpc.UnaryServerInterceptor, ctx context.Context) error {
	t.Helper()
	_, err := interceptor(ctx, &dispatchpb.SubmitTaskRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: dispatchpb.DispatchService_SubmitTask_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &dispatchpb.SubmitTaskResponse{}, nil
	})
	return err
}

func TestAuth_ViewerCannotSubmit(t *testing.T) {
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		mustHealthyChecker(t),
		CatalogOf,
		grpcauth.ProtoTenantID,
	)
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	err := callSubmit(t, interceptor, ctx)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestAuth_OperatorCanSubmit(t *testing.T) {
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		mustHealthyChecker(t),
		CatalogOf,
		grpcauth.ProtoTenantID,
	)
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "operator"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	if err := callSubmit(t, interceptor, ctx); err != nil {
		t.Fatal(err)
	}
}

func TestAuth_StaleDeniesSubmit(t *testing.T) {
	c, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter:        1 * time.Minute,
		StaleAfter:          10 * time.Minute,
		DenyWritesWhenStale: true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	synced := time.Now().Add(-2 * time.Minute)
	if err := c.ReplacePolicies(dispatchPolicies(), synced); err != nil {
		t.Fatal(err)
	}
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		c,
		CatalogOf,
		grpcauth.ProtoTenantID,
	)
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "operator"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	err = callSubmit(t, interceptor, ctx)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestAuth_ColdStartSafetyNet(t *testing.T) {
	c, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter: 1 * time.Minute,
		StaleAfter:   5 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		c,
		CatalogOf,
		grpcauth.ProtoTenantID,
	)

	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "operator"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	if err := callSubmit(t, interceptor, ctx); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("operator cold start: %v", err)
	}

	md = metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "admin"))
	ctx = metadata.NewIncomingContext(context.Background(), md)
	if err := callSubmit(t, interceptor, ctx); err != nil {
		t.Fatalf("admin cold start: %v", err)
	}
}

func TestAuth_GetTaskReadAllowedForViewer(t *testing.T) {
	interceptor := grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		mustHealthyChecker(t),
		CatalogOf,
		grpcauth.ProtoTenantID,
	)
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, &dispatchpb.GetTaskRequest{TenantID: "default", TaskID: "t1"}, &grpc.UnaryServerInfo{
		FullMethod: dispatchpb.DispatchService_GetTask_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &dispatchpb.GetTaskResponse{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
