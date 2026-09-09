package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryClientTimeoutInterceptor_AddsDeadlineWhenAbsent(t *testing.T) {
	t.Parallel()

	var gotDeadlineOK bool
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		_, gotDeadlineOK = ctx.Deadline()
		return nil
	}

	interceptor := UnaryClientTimeoutInterceptor(5 * time.Second)
	if err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotDeadlineOK {
		t.Fatal("expected a deadline to be added when ctx had none")
	}
}

func TestUnaryClientTimeoutInterceptor_PreservesExistingDeadline(t *testing.T) {
	t.Parallel()

	want := time.Now().Add(2 * time.Hour)
	ctx, cancel := context.WithDeadline(context.Background(), want)
	defer cancel()

	var got time.Time
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		got, _ = ctx.Deadline()
		return nil
	}

	// A much shorter timeout must NOT override an already-set deadline.
	interceptor := UnaryClientTimeoutInterceptor(1 * time.Second)
	if err := interceptor(ctx, "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("got deadline %v, want unchanged %v", got, want)
	}
}

func TestUnaryClientTimeoutInterceptor_ZeroDisables(t *testing.T) {
	t.Parallel()

	var gotDeadlineOK bool
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		_, gotDeadlineOK = ctx.Deadline()
		return nil
	}

	interceptor := UnaryClientTimeoutInterceptor(0)
	if err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDeadlineOK {
		t.Fatal("expected no deadline to be added when timeout is 0")
	}
}

func TestUnaryClientBreakerInterceptor_NilPassthrough(t *testing.T) {
	t.Parallel()

	called := false
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		called = true
		return nil
	}

	interceptor := UnaryClientBreakerInterceptor(nil)
	if err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected invoker to be called when breaker is nil")
	}
}

func TestUnaryClientBreakerInterceptor_OpenShortCircuits(t *testing.T) {
	t.Parallel()

	cb := NewBreaker[any](BreakerConfig{
		Enabled:             true,
		Name:                "test-grpc-dep",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
	})

	// Trip the breaker with one failing call.
	interceptor := UnaryClientBreakerInterceptor(cb)
	failingInvoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return errors.New("boom")
	}
	if err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, failingInvoker); err == nil {
		t.Fatal("expected the first (failing) call to propagate its error")
	}

	// Second call: breaker should now be open; invoker must NOT be called.
	called := false
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		called = true
		return nil
	}
	err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, invoker)
	if called {
		t.Fatal("expected invoker NOT to be called once breaker is open")
	}
	if !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("got err=%v, want ErrDependencyUnavailable", err)
	}
}

func TestGRPCIsSuccessful(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, true},
		{"context canceled", context.Canceled, true},
		{"not found", status.Error(codes.NotFound, "missing"), true},
		{"failed precondition", status.Error(codes.FailedPrecondition, "bad state"), true},
		{"invalid argument", status.Error(codes.InvalidArgument, "bad input"), true},
		{"already exists", status.Error(codes.AlreadyExists, "dup"), true},
		{"permission denied", status.Error(codes.PermissionDenied, "no"), true},
		{"unauthenticated", status.Error(codes.Unauthenticated, "no"), true},
		{"unavailable", status.Error(codes.Unavailable, "down"), false},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "slow"), false},
		{"internal", status.Error(codes.Internal, "oops"), false},
		{"unknown", status.Error(codes.Unknown, "?"), false},
		{"resource exhausted", status.Error(codes.ResourceExhausted, "rate limited"), false},
		{"non-status error", errors.New("plain error"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GRPCIsSuccessful(tc.err)
			if got != tc.want {
				t.Fatalf("GRPCIsSuccessful(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
