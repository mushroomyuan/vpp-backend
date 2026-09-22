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
	"google.golang.org/protobuf/types/known/timestamppb"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type fakeTelemetryServer struct {
	telemetrypb.UnimplementedTelemetryServiceServer
	resp  *telemetrypb.QueryAggregationResponse
	fail  bool
	calls atomic.Int64
	last  atomic.Pointer[telemetrypb.QueryAggregationRequest]
}

func (s *fakeTelemetryServer) QueryAggregation(
	_ context.Context, req *telemetrypb.QueryAggregationRequest,
) (*telemetrypb.QueryAggregationResponse, error) {
	s.calls.Add(1)
	s.last.Store(req)
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

func TestClient_QueryAggregation_ConvertsResponseAndRequestsAvgLast(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
	avg := 42.5
	last := 40.0
	srv := &fakeTelemetryServer{
		resp: &telemetrypb.QueryAggregationResponse{
			Points: []*telemetrypb.AggregatedPoint{
				{
					CUCode:      "cu-1",
					MetricName:  "active_power_kw",
					WindowStart: timestamppb.New(start),
					Avg:         &avg,
					Last:        &last,
				},
				{
					// Dropped: predictors cannot place a bucket with no window.
					Avg: &avg,
				},
			},
		},
	}
	dialer, cleanup := newBufconnTelemetryServer(t, srv)
	defer cleanup()

	client, err := NewClient(Config{
		Addr:        "passthrough:///bufforecast-telemetry",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	got, err := client.QueryAggregation(context.Background(), "tenant-1", "cu-1", "active_power_kw", start, end, 900)
	if err != nil {
		t.Fatalf("QueryAggregation: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(points) = %d, want 1 (nil WindowStart dropped)", len(got))
	}
	if !got[0].Timestamp.Equal(start) {
		t.Errorf("Timestamp = %s, want %s", got[0].Timestamp, start)
	}
	if got[0].Avg == nil || *got[0].Avg != avg {
		t.Errorf("Avg = %v, want %v", got[0].Avg, avg)
	}
	if got[0].Last == nil || *got[0].Last != last {
		t.Errorf("Last = %v, want %v", got[0].Last, last)
	}

	req := srv.last.Load()
	if req == nil {
		t.Fatal("expected QueryAggregation to be invoked")
	}
	if req.GetTenantID() != "tenant-1" || req.GetCUCode() != "cu-1" || req.GetMetricName() != "active_power_kw" {
		t.Errorf("identity = (%s,%s,%s)", req.GetTenantID(), req.GetCUCode(), req.GetMetricName())
	}
	if req.GetStepSeconds() != 900 {
		t.Errorf("StepSeconds = %d, want 900", req.GetStepSeconds())
	}
	if !req.GetStartTime().AsTime().Equal(start) || !req.GetEndTime().AsTime().Equal(end) {
		t.Errorf("window = [%s, %s]", req.GetStartTime().AsTime(), req.GetEndTime().AsTime())
	}
	funcs := req.GetFunctions()
	if len(funcs) != 2 ||
		funcs[0] != telemetrypb.AggFunction_AGG_FUNCTION_AVG ||
		funcs[1] != telemetrypb.AggFunction_AGG_FUNCTION_LAST {
		t.Errorf("Functions = %v, want [AVG, LAST]", funcs)
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv := &fakeTelemetryServer{fail: true}
	dialer, cleanup := newBufconnTelemetryServer(t, srv)
	defer cleanup()

	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-forecast->telemetry",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})

	client, err := NewClient(Config{
		Addr:        "passthrough:///bufforecast-telemetry-breaker",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Breaker:     breaker,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx := context.Background()
	start := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	if _, err := client.QueryAggregation(ctx, "tenant-1", "cu-1", "kw", start, end, 900); err == nil {
		t.Fatal("expected the first call to surface the underlying RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 real RPC after the first call, got %d", got)
	}

	_, err = client.QueryAggregation(ctx, "tenant-1", "cu-1", "kw", start, end, 900)
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
