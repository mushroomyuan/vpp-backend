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

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
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

func telemetryPolicies() []authz.PolicyRule {
	return []authz.PolicyRule{
		{"viewer", "telemetry:telemetry", "read"},
		{"viewer", "telemetry:snapshots", "read"},
		{"viewer", "telemetry:aggregation", "read"},
		{"operator", "telemetry:telemetry", "read"},
		{"operator", "telemetry:snapshots", "read"},
		{"operator", "telemetry:aggregation", "read"},
		{"admin", "telemetry:telemetry", "read"},
		{"admin", "telemetry:snapshots", "read"},
		{"admin", "telemetry:aggregation", "read"},
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
	if err := c.ReplacePolicies(telemetryPolicies(), time.Now()); err != nil {
		t.Fatal(err)
	}
	return c
}

func withPEP(c *authz.Checker) grpc.UnaryServerInterceptor {
	return WithMachineBypass(grpcauth.UnaryServerInterceptor(
		grpcauth.Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		c,
		CatalogOf,
		grpcauth.ProtoTenantID,
	))
}

func TestCatalogOf(t *testing.T) {
	cases := []struct {
		method, obj, act string
		ok               bool
	}{
		{telemetrypb.TelemetryService_QueryTelemetry_FullMethodName, "telemetry:telemetry", "read", true},
		{telemetrypb.TelemetryService_GetSnapshot_FullMethodName, "telemetry:snapshots", "read", true},
		{telemetrypb.TelemetryService_GetFleetSnapshot_FullMethodName, "telemetry:snapshots", "read", true},
		{telemetrypb.TelemetryService_QueryAggregation_FullMethodName, "telemetry:aggregation", "read", true},
		{telemetrypb.TelemetryService_IngestTelemetry_FullMethodName, "", "", false},
	}
	for _, tc := range cases {
		obj, act, ok := CatalogOf(tc.method)
		if obj != tc.obj || act != tc.act || ok != tc.ok {
			t.Fatalf("%s: got (%q,%q,%v)", tc.method, obj, act, ok)
		}
	}
	if !SkipUserAuth(telemetrypb.TelemetryService_IngestTelemetry_FullMethodName) {
		t.Fatal("ingest should skip user auth")
	}
	cat := AuthzCatalog("default", "default/vpp-rbac")
	if cat.Service != "telemetry" || len(cat.Entries) != 3 {
		t.Fatalf("%+v", cat)
	}
}

func TestAuth_IngestBypassesUserinfo(t *testing.T) {
	interceptor := withPEP(mustHealthyChecker(t))
	_, err := interceptor(context.Background(), &telemetrypb.IngestTelemetryRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: telemetrypb.TelemetryService_IngestTelemetry_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &telemetrypb.IngestTelemetryResponse{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuth_QueryRequiresUserinfo(t *testing.T) {
	interceptor := withPEP(mustHealthyChecker(t))
	_, err := interceptor(context.Background(), &telemetrypb.QueryTelemetryRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: telemetrypb.TelemetryService_QueryTelemetry_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &telemetrypb.QueryTelemetryResponse{}, nil
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestAuth_ViewerCanQuery(t *testing.T) {
	interceptor := withPEP(mustHealthyChecker(t))
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, &telemetrypb.QueryTelemetryRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: telemetrypb.TelemetryService_QueryTelemetry_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &telemetrypb.QueryTelemetryResponse{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuth_TenantMismatch(t *testing.T) {
	interceptor := withPEP(mustHealthyChecker(t))
	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err := interceptor(ctx, &telemetrypb.GetFleetSnapshotRequest{TenantID: "other"}, &grpc.UnaryServerInfo{
		FullMethod: telemetrypb.TelemetryService_GetFleetSnapshot_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return &telemetrypb.GetFleetSnapshotResponse{}, nil
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestAuth_ColdStartSafetyNet(t *testing.T) {
	c, err := authz.NewCheckerWithMetrics(authz.Config{
		HealthyAfter: 5 * time.Minute,
		StaleAfter:   30 * time.Minute,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	interceptor := withPEP(c)

	md := metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	_, err = interceptor(ctx, &telemetrypb.GetSnapshotRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: telemetrypb.TelemetryService_GetSnapshot_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return nil, nil
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("viewer cold start: %v", err)
	}

	md = metadata.Pairs(grpcauth.MetadataUserinfoKey, encodeUserinfo(t, "default", "admin"))
	ctx = metadata.NewIncomingContext(context.Background(), md)
	_, err = interceptor(ctx, &telemetrypb.GetSnapshotRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: telemetrypb.TelemetryService_GetSnapshot_FullMethodName,
	}, func(context.Context, any) (any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("admin cold start: %v", err)
	}
}
