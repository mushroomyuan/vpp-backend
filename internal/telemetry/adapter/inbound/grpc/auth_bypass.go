package grpc

import (
	"context"

	"google.golang.org/grpc"
)

// WithMachineBypass wraps a user PEP so SkipUserAuth methods (e.g. Ingest)
// never require x-userinfo / Casbin.
func WithMachineBypass(pep grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if SkipUserAuth(info.FullMethod) {
			return handler(ctx, req)
		}
		return pep(ctx, req, info, handler)
	}
}
