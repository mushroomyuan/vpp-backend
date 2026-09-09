package gatewaygrpc

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// alwaysFailGatewayServer implements gatewaypb.GatewayServiceServer and
// always returns codes.Unavailable, tracking how many times it was actually
// invoked so tests can prove the breaker short-circuits without hitting the
// wire once it is Open.
type alwaysFailGatewayServer struct {
	gatewaypb.UnimplementedGatewayServiceServer
	calls atomic.Int64
}

func (s *alwaysFailGatewayServer) ExecuteCommand(
	ctx context.Context,
	req *gatewaypb.ExecuteCommandRequest,
) (*gatewaypb.ExecuteCommandResponse, error) {
	s.calls.Add(1)
	return nil, status.Error(codes.Unavailable, "simulated dependency failure")
}

func newBufconnGatewayServer(t *testing.T) (*alwaysFailGatewayServer, func(context.Context, string) (net.Conn, error), func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := &alwaysFailGatewayServer{}
	grpcSrv := platformserver.NewGRPCServer()
	gatewaypb.RegisterGatewayServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	cleanup := func() { grpcSrv.Stop() }
	return srv, dialer, cleanup
}

func testCommand() *model.ControlCommand {
	return &model.ControlCommand{
		ID:       "cmd-1",
		TenantID: "tenant-1",
		CUCode:   "cu-1",
		PointKey: "point-1",
		Value:    model.BoolCommandValue(true),
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv, dialer, cleanup := newBufconnGatewayServer(t)
	defer cleanup()

	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-dispatch->gateway",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})

	client, err := NewClient(Config{
		Addr:        "passthrough:///bufgateway",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Breaker:     breaker,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// First call: the underlying RPC actually runs and fails, tripping the
	// breaker (ConsecutiveFailures: 1). ExecuteCommand maps this to an error
	// (application treats it as a rejection), not to GatewayRejected.
	if _, err := client.ExecuteCommand(ctx, testCommand()); err == nil {
		t.Fatal("expected the first call to surface the underlying RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 real RPC after the first call, got %d", got)
	}

	// Second call: breaker is now Open. The client must short-circuit
	// without making a real RPC, and surface ErrDependencyUnavailable.
	_, err = client.ExecuteCommand(ctx, testCommand())
	if err == nil {
		t.Fatal("expected an error once the breaker is open")
	}
	if !errors.Is(err, resilience.ErrDependencyUnavailable) {
		t.Fatalf("got err=%v, want it to wrap resilience.ErrDependencyUnavailable", err)
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected NO additional real RPCs once the breaker is open, got %d calls", got)
	}
}

func TestClient_TimeoutInterceptor_BoundsUnboundedCalls(t *testing.T) {
	t.Parallel()

	// Listener with no server Serve()-ing it: dialing succeeds lazily (gRPC
	// dials lazily), but any RPC attempt will hang until it is bounded by a
	// deadline. This proves cfg.Timeout actually bounds the call.
	lis := bufconn.Listen(1024 * 1024)
	defer func() { _ = lis.Close() }()
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	client, err := NewClient(Config{
		Addr:        "passthrough:///bufgateway-hang",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Timeout:     100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	start := time.Now()
	_, err = client.ExecuteCommand(context.Background(), testCommand())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from a call with no server listening")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected the call to be bounded by the configured timeout, took %v", elapsed)
	}
}
