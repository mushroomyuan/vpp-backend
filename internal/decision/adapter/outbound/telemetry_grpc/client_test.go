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

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type fakeTelemetryServer struct {
	telemetrypb.UnimplementedTelemetryServiceServer
	resp    *telemetrypb.GetSnapshotsResponse
	lastReq *telemetrypb.GetSnapshotsRequest
	fail    bool
	calls   atomic.Int64
}

func (s *fakeTelemetryServer) GetSnapshots(
	_ context.Context, req *telemetrypb.GetSnapshotsRequest,
) (*telemetrypb.GetSnapshotsResponse, error) {
	s.calls.Add(1)
	s.lastReq = req
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
	return dialer, func() { grpcSrv.Stop() }
}

func TestClient_GetSnapshots_KeepsPerMetricQuality(t *testing.T) {
	t.Parallel()

	observed := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	updated := observed.Add(time.Second)
	srv := &fakeTelemetryServer{
		resp: &telemetrypb.GetSnapshotsResponse{
			Snapshots: []*telemetrypb.Snapshot{
				{
					CUCode: "cu-1",
					Metrics: []*telemetrypb.MetricState{{
						MetricID:    string(contracts.MetricEnergyStorageStateOfCharge),
						DoubleValue: 42.5,
						ObservedAt:  timestamppb.New(observed),
						Quality:     telemetrypb.QualityStatus_QUALITY_STATUS_GOOD,
					}},
					UpdatedAt: timestamppb.New(updated),
					Stale:     false,
				},
				{
					CUCode: "cu-2",
					Metrics: []*telemetrypb.MetricState{{
						MetricID:    string(contracts.MetricElectricalActivePower),
						DoubleValue: -3,
						ObservedAt:  timestamppb.New(observed),
						Quality:     telemetrypb.QualityStatus_QUALITY_STATUS_BAD,
					}},
					UpdatedAt: timestamppb.New(updated),
					Stale:     true,
				},
			},
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

	snaps, err := client.GetSnapshots(context.Background(), port.SnapshotQuery{
		TenantID:  "tenant-1",
		CUCodes:   []string{"cu-1", "cu-2"},
		MetricIDs: []contracts.MetricID{contracts.MetricEnergyStorageStateOfCharge, contracts.MetricElectricalActivePower},
		StaleAge:  2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}
	if snaps[0].CUCode != "cu-1" || snaps[0].Stale || len(snaps[0].Metrics) != 1 {
		t.Fatalf("first snapshot = %+v", snaps[0])
	}
	sample := snaps[0].Metrics[0]
	if sample.MetricID != contracts.MetricEnergyStorageStateOfCharge || sample.Value != 42.5 || sample.Quality != port.QualityGood {
		t.Fatalf("first sample = %+v", sample)
	}
	if !sample.ObservedAt.Equal(observed) || !snaps[0].UpdatedAt.Equal(updated) {
		t.Fatalf("times observed=%s updated=%s", sample.ObservedAt, snaps[0].UpdatedAt)
	}
	if !snaps[1].Stale || snaps[1].Metrics[0].Quality != port.QualityBad || snaps[1].Metrics[0].Value != -3 {
		t.Fatalf("bad sample was collapsed: %+v", snaps[1])
	}

	req := srv.lastReq
	if req.GetTenantID() != "tenant-1" || len(req.GetCUCodes()) != 2 || len(req.GetMetricIDs()) != 2 {
		t.Fatalf("request = %+v", req)
	}
	if req.GetStaleAgeSeconds() != 120 {
		t.Fatalf("StaleAgeSeconds = %d, want 120", req.GetStaleAgeSeconds())
	}
	if req.GetMetricIDs()[0] != string(contracts.MetricEnergyStorageStateOfCharge) {
		t.Fatalf("metric ids = %v", req.GetMetricIDs())
	}
}

func TestClient_GetSnapshots_RejectsEmptyCUListWithoutRPC(t *testing.T) {
	t.Parallel()

	srv := &fakeTelemetryServer{}
	dialer, cleanup := newBufconnTelemetryServer(t, srv)
	defer cleanup()
	client, err := NewClient(Config{
		Addr:        "passthrough:///buftelemetry-empty",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.GetSnapshots(context.Background(), port.SnapshotQuery{
		TenantID:  "tenant-1",
		MetricIDs: []contracts.MetricID{contracts.MetricEnergyStorageStateOfCharge},
	})
	if err == nil {
		t.Fatal("expected an error for an empty CU list")
	}
	if got := srv.calls.Load(); got != 0 {
		t.Fatalf("expected no RPC, got %d", got)
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv := &fakeTelemetryServer{fail: true}
	dialer, cleanup := newBufconnTelemetryServer(t, srv)
	defer cleanup()

	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-decision->telemetry",
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

	query := port.SnapshotQuery{
		TenantID:  "tenant-1",
		CUCodes:   []string{"cu-1"},
		MetricIDs: []contracts.MetricID{contracts.MetricEnergyStorageStateOfCharge},
	}
	if _, err := client.GetSnapshots(context.Background(), query); err == nil {
		t.Fatal("expected the first call to surface the RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
	_, err = client.GetSnapshots(context.Background(), query)
	if !errors.Is(err, resilience.ErrDependencyUnavailable) {
		t.Fatalf("err = %v, want ErrDependencyUnavailable", err)
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("breaker open still called telemetry, calls = %d", got)
	}
}

func TestNewClient_RequiresAddr(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected an error when Addr is empty")
	}
}
