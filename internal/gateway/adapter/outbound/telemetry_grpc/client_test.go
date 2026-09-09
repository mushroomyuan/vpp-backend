package telemetrygrpc

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

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// alwaysFailTelemetryServer implements telemetrypb.TelemetryServiceServer and
// always returns codes.Unavailable, tracking how many times it was actually
// invoked so tests can prove the breaker short-circuits without hitting the
// wire once it is Open.
type alwaysFailTelemetryServer struct {
	telemetrypb.UnimplementedTelemetryServiceServer
	calls atomic.Int64
}

func (s *alwaysFailTelemetryServer) IngestTelemetry(
	ctx context.Context,
	req *telemetrypb.IngestTelemetryRequest,
) (*telemetrypb.IngestTelemetryResponse, error) {
	s.calls.Add(1)
	return nil, status.Error(codes.Unavailable, "simulated dependency failure")
}

func newBufconnTelemetryServer(t *testing.T) (*alwaysFailTelemetryServer, func(context.Context, string) (net.Conn, error), func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := &alwaysFailTelemetryServer{}
	grpcSrv := platformserver.NewGRPCServer()
	telemetrypb.RegisterTelemetryServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	cleanup := func() { grpcSrv.Stop() }
	return srv, dialer, cleanup
}

func testTelemetry() *model.StandardTelemetry {
	return &model.StandardTelemetry{
		TenantID:  "tenant-1",
		CUCode:    "cu-1",
		Timestamp: time.Now(),
		Metrics: []model.MetricValue{
			{Name: "power_kw", Value: 1.23, Type: model.MetricTypeAnalog, Quality: model.QualityGood},
		},
	}
}

func TestTelemetryGRPCClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv, dialer, cleanup := newBufconnTelemetryServer(t)
	defer cleanup()

	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-gateway->telemetry",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})

	client, err := NewTelemetryGRPCClient(Config{
		Addr:        "passthrough:///buftelemetry",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Breaker:     breaker,
	})
	if err != nil {
		t.Fatalf("NewTelemetryGRPCClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()

	// First call: the underlying RPC actually runs and fails, tripping the
	// breaker (ConsecutiveFailures: 1).
	if err := client.Ingest(ctx, testTelemetry()); err == nil {
		t.Fatal("expected the first call to surface the underlying RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 real RPC after the first call, got %d", got)
	}

	// Second call: breaker is now Open. The client must short-circuit
	// without making a real RPC, and surface ErrDependencyUnavailable.
	err = client.Ingest(ctx, testTelemetry())
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

func TestTelemetryGRPCClient_TimeoutInterceptor_BoundsUnboundedCalls(t *testing.T) {
	t.Parallel()

	// Listener with no server Serve()-ing it: dialing succeeds lazily (gRPC
	// dials lazily), but any RPC attempt will hang until it is bounded by a
	// deadline. This proves cfg.Timeout actually bounds the call.
	lis := bufconn.Listen(1024 * 1024)
	defer func() { _ = lis.Close() }()
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	client, err := NewTelemetryGRPCClient(Config{
		Addr:        "passthrough:///buftelemetry-hang",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Timeout:     100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewTelemetryGRPCClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	start := time.Now()
	err = client.Ingest(context.Background(), testTelemetry())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error from a call with no server listening")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected the call to be bounded by the configured timeout, took %v", elapsed)
	}
}
