// Package grpcauth provides the gRPC policy-enforcement middleware for
// end-user requests authenticated by the trusted OIDC ingress.
package grpcauth

import (
	"context"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/authn"
	"github.com/mushroomyuan/vpp-backend/platform/authz"
	"github.com/mushroomyuan/vpp-backend/platform/identity"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataUserinfoKey carries the same Casdoor payload as APISIX X-Userinfo.
const MetadataUserinfoKey = "x-userinfo"

type Config struct {
	// TrustProxyHeaders requires valid ingress userinfo and authorization.
	// False bypasses the PEP for local debugging.
	TrustProxyHeaders bool
}

// MethodCatalog maps a gRPC full method to a logical resource and action.
type MethodCatalog func(fullMethod string) (resource, action string, ok bool)

// TenantIDFromRequest extracts a tenant ID from a protobuf request.
type TenantIDFromRequest func(req any) (tenantID string, ok bool)

// ProtoTenantID extracts TenantID through the conventional protobuf getter.
func ProtoTenantID(req any) (string, bool) {
	type tenantGetter interface {
		GetTenantID() string
	}
	getter, ok := req.(tenantGetter)
	if !ok {
		return "", false
	}
	tenantID := strings.TrimSpace(getter.GetTenantID())
	if tenantID == "" {
		return "", false
	}
	return tenantID, true
}

// UnaryServerInterceptor enforces identity, tenant binding, and permission checks.
func UnaryServerInterceptor(
	cfg Config,
	parsePrincipal authn.PrincipalParser,
	checker authz.PermissionChecker,
	catalog MethodCatalog,
	tenantOf TenantIDFromRequest,
) grpc.UnaryServerInterceptor {
	if tenantOf == nil {
		tenantOf = ProtoTenantID
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if !cfg.TrustProxyHeaders {
			return handler(ctx, req)
		}
		if checker == nil {
			return nil, status.Error(codes.Internal, "authorization checker not configured")
		}
		if parsePrincipal == nil {
			return nil, status.Error(codes.Internal, "principal parser not configured")
		}
		if catalog == nil {
			return nil, status.Error(codes.Internal, "authorization catalog not configured")
		}

		md, _ := metadata.FromIncomingContext(ctx)
		principal, err := parsePrincipal(firstMetadata(md, MetadataUserinfoKey))
		if err != nil {
			return nil, status.Error(codes.Unauthenticated,
				"missing or invalid x-userinfo metadata (enable OIDC ingress or set auth.trust-proxy-headers: false for local debug)")
		}

		if requestTenant, ok := tenantOf(req); ok && requestTenant != principal.TenantID {
			return nil, status.Error(codes.PermissionDenied,
				"tenant mismatch: request TenantID does not match principal TenantID")
		}

		resource, action, ok := catalog(info.FullMethod)
		if !ok || resource == "" || action == "" {
			return nil, status.Error(codes.PermissionDenied, "forbidden: role cannot perform this action")
		}

		decision, err := checker.Allow(ctx, principal, resource, action)
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"method":   info.FullMethod,
				"resource": resource,
				"action":   action,
				"user":     principal.Username,
			}).Error("authz Allow failed")
			return nil, status.Error(codes.PermissionDenied, "forbidden: authorization error")
		}
		if !decision.Allowed {
			message := "forbidden: role cannot perform this action"
			if decision.Degraded {
				message = "forbidden: authorization unavailable or policy stale"
			}
			return nil, status.Error(codes.PermissionDenied, message)
		}
		if decision.Degraded {
			logrus.WithFields(logrus.Fields{
				"method":   info.FullMethod,
				"resource": resource,
				"action":   action,
				"user":     principal.Username,
				"roles":    principal.Roles,
			}).Warn("authz decision made in degraded mode")
		}

		return handler(identity.NewContext(ctx, principal), req)
	}
}

func firstMetadata(md metadata.MD, key string) string {
	if md == nil {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
