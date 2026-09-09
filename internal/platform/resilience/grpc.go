package resilience

import (
	"context"
	"errors"
	"time"

	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryClientTimeoutInterceptor bounds outbound calls that don't already
// carry a context deadline. Callers that set their own deadline (or
// cancellation) are left untouched — this only fills the gap.
func UnaryClientTimeoutInterceptor(d time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if d > 0 {
			if _, ok := ctx.Deadline(); !ok {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, d)
				defer cancel()
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// UnaryClientBreakerInterceptor short-circuits calls once cb is Open. A nil
// cb (breaker disabled) is a no-op passthrough.
func UnaryClientBreakerInterceptor(cb *gobreaker.CircuitBreaker[any]) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if cb == nil {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		_, err := cb.Execute(func() (any, error) {
			return nil, invoker(ctx, method, req, reply, cc, opts...)
		})
		return unwrapBreakerErr(err)
	}
}

// GRPCIsSuccessful classifies a gRPC error for breaker bookkeeping:
// business-level rejections mean the dependency is healthy and correctly
// rejected the request, so they must NOT count as breaker failures. Only
// transport/availability failures (Unavailable, DeadlineExceeded, Internal,
// Unknown, ResourceExhausted, ...) count as failures.
func GRPCIsSuccessful(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return true
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.NotFound, codes.FailedPrecondition, codes.InvalidArgument,
		codes.AlreadyExists, codes.PermissionDenied, codes.Unauthenticated:
		return true
	default:
		return false
	}
}
