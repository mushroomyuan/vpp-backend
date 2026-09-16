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
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// fakeTelemetryServer implements telemetrypb.TelemetryServiceServer with a
// canned GetSnapshot response, or a simulated failure when configured to.
type fakeTelemetryServer struct {
	telemetrypb.UnimplementedTelemetryServiceServer
	resp  *telemetrypb.Snapshot
	fail  bool
	calls atomic.Int64
}

func (s *fakeTelemetryServer) GetSnapshot(
	_ context.Context, _ *telemetrypb.GetSnapshotRequest,
) (*telemetrypb.Snapshot, error) {
	s.calls.Add(1)
	if s.fail {
		return nil, status.Error(codes.Unavailable, "simulated dependency failure")
	}
	return s.resp, nil
}

func newBufconnTelemetryServer(t *testing.T, srv *fakeTelemetryServer) (func(context.Context, string) (net.Conn, error), func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := platformserver.NewGRPCServer()
	telemetrypb.RegisterTelemetryServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	cleanup := func() { grpcSrv.Stop() }
	return dialer, cleanup
}

func TestClient_GetSnapshot_ConvertsResponse(t *testing.T) {
	t.Parallel()

	srv := &fakeTelemetryServer{
		resp: &telemetrypb.Snapshot{
			CUCode:  "cu-1",
			Metrics: map[string]float64{"soc": 42.5},
			Stale:   false,
		},
	}
	dialer, cleanup := newBufconnTelemetryServer(t, srv)
	defer cleanup()

	client, err := NewClient(Config{
		Addr:        "passthrough:///buftelemetry",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	snap, err := client.GetSnapshot(context.Background(), "tenant-1", "cu-1")
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.CUCode != "cu-1" {
		t.Errorf("CUCode = %q, want %q", snap.CUCode, "cu-1")
	}
	if snap.Metrics["soc"] != 42.5 {
		t.Errorf("Metrics[soc] = %v, want 42.5", snap.Metrics["soc"])
	}
	if snap.Stale {
		t.Error("expected Stale = false")
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv := &fakeTelemetryServer{fail: true}
	dialer, cleanup := newBufconnTelemetryServer(t, srv)
	defer cleanup()

	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-optimization->telemetry",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})

	client, err := NewClient(Config{
		Addr:        "passthrough:///buftelemetry-breaker",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Breaker:     breaker,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	if _, err := client.GetSnapshot(ctx, "tenant-1", "cu-1"); err == nil {
		t.Fatal("expected the first call to surface the underlying RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 real RPC after the first call, got %d", got)
	}

	_, err = client.GetSnapshot(ctx, "tenant-1", "cu-1")
	if !errors.Is(err, resilience.ErrDependencyUnavailable) {
		t.Fatalf("got err=%v, want it to wrap resilience.ErrDependencyUnavailable", err)
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected NO additional real RPCs once the breaker is open, got %d calls", got)
	}
}

func TestNewClient_RequiresAddr(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected an error when Addr is empty")
	}
}
