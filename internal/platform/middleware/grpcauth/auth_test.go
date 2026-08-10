package grpcauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/mushroomyuan/vpp-backend/platform/authn/casdoor"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type stubChecker struct {
	allowed  bool
	degraded bool
	err      error
}

func (s stubChecker) Allow(context.Context, identity.Principal, string, string) (authz.Decision, error) {
	return authz.Decision{Allowed: s.allowed, Degraded: s.degraded}, s.err
}

type tenantRequest struct{ TenantID string }

func (r *tenantRequest) GetTenantID() string { return r.TenantID }

func encodeUserinfo(t *testing.T, tenant, role string) string {
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
	return base64.StdEncoding.EncodeToString(raw)
}

func catalogOK(string) (string, string, bool) {
	return "dispatch:tasks", "submit", true
}

func invoke(t *testing.T, interceptor grpc.UnaryServerInterceptor, ctx context.Context, req any) error {
	t.Helper()
	_, err := interceptor(ctx, req, &grpc.UnaryServerInfo{
		FullMethod: "/dispatchpb.DispatchService/SubmitTask",
	}, func(context.Context, any) (any, error) {
		return "ok", nil
	})
	return err
}

func TestBypassWhenTrustFalse(t *testing.T) {
	interceptor := UnaryServerInterceptor(Config{}, nil, nil, nil, nil)
	if err := invoke(t, interceptor, context.Background(), &tenantRequest{TenantID: "default"}); err != nil {
		t.Fatal(err)
	}
}

func TestRequireUserinfo(t *testing.T) {
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		stubChecker{allowed: true},
		catalogOK,
		ProtoTenantID,
	)
	err := invoke(t, interceptor, context.Background(), &tenantRequest{TenantID: "default"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestTenantMismatch(t *testing.T) {
	md := metadata.Pairs(MetadataUserinfoKey, encodeUserinfo(t, "default", "admin"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		stubChecker{allowed: true},
		catalogOK,
		ProtoTenantID,
	)
	err := invoke(t, interceptor, ctx, &tenantRequest{TenantID: "other"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestDeny(t *testing.T) {
	md := metadata.Pairs(MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		stubChecker{},
		catalogOK,
		ProtoTenantID,
	)
	err := invoke(t, interceptor, ctx, &tenantRequest{TenantID: "default"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestDegradedDenyMessage(t *testing.T) {
	md := metadata.Pairs(MetadataUserinfoKey, encodeUserinfo(t, "default", "viewer"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		stubChecker{degraded: true},
		catalogOK,
		ProtoTenantID,
	)
	err := invoke(t, interceptor, ctx, &tenantRequest{TenantID: "default"})
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied || st.Message() != "forbidden: authorization unavailable or policy stale" {
		t.Fatalf("status=%v", st)
	}
}

func TestAllowInjectsPrincipal(t *testing.T) {
	md := metadata.Pairs(MetadataUserinfoKey, encodeUserinfo(t, "default", "admin"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		stubChecker{allowed: true},
		catalogOK,
		ProtoTenantID,
	)

	var got identity.Principal
	_, err := interceptor(ctx, &tenantRequest{TenantID: "default"}, &grpc.UnaryServerInfo{
		FullMethod: "/dispatchpb.DispatchService/SubmitTask",
	}, func(ctx context.Context, _ any) (any, error) {
		var ok bool
		got, ok = identity.FromContext(ctx)
		if !ok {
			t.Fatal("principal missing")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TenantID != "default" || !got.HasRole("admin") {
		t.Fatalf("principal=%+v", got)
	}
}

func TestNilChecker(t *testing.T) {
	md := metadata.Pairs(MetadataUserinfoKey, encodeUserinfo(t, "default", "admin"))
	ctx := metadata.NewIncomingContext(context.Background(), md)
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		casdoor.ParseUserinfo,
		nil,
		catalogOK,
		ProtoTenantID,
	)
	err := invoke(t, interceptor, ctx, &tenantRequest{TenantID: "default"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func TestNilPrincipalParser(t *testing.T) {
	interceptor := UnaryServerInterceptor(
		Config{TrustProxyHeaders: true},
		nil,
		stubChecker{allowed: true},
		catalogOK,
		ProtoTenantID,
	)
	err := invoke(t, interceptor, context.Background(), &tenantRequest{TenantID: "default"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code=%v", status.Code(err))
	}
}
