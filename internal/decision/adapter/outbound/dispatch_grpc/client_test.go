package dispatchgrpc

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

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	appport "github.com/mushroomyuan/vpp-backend/decision/application/port"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

type fakeDispatchServer struct {
	dispatchpb.UnimplementedDispatchServiceServer
	lastReq *dispatchpb.SubmitTaskRequest
	fail    bool
	calls   atomic.Int64
}

func (s *fakeDispatchServer) SubmitTask(
	_ context.Context, req *dispatchpb.SubmitTaskRequest,
) (*dispatchpb.SubmitTaskResponse, error) {
	s.calls.Add(1)
	if s.fail {
		return nil, status.Error(codes.Unavailable, "simulated dependency failure")
	}
	s.lastReq = req
	return &dispatchpb.SubmitTaskResponse{TaskID: "task-1", Status: "running"}, nil
}

func newBufconnDispatchServer(t *testing.T, srv *fakeDispatchServer) (func(context.Context, string) (net.Conn, error), func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	grpcSrv := platformserver.NewGRPCServer()
	dispatchpb.RegisterDispatchServiceServer(grpcSrv, srv)
	go func() { _ = grpcSrv.Serve(lis) }()
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	return dialer, func() { grpcSrv.Stop() }
}

func TestClient_SubmitTask_WritesMetricIDIntoPointKey(t *testing.T) {
	t.Parallel()

	srv := &fakeDispatchServer{}
	dialer, cleanup := newBufconnDispatchServer(t, srv)
	defer cleanup()
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufdispatch",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	result, err := client.SubmitTask(context.Background(), appport.Task{
		TenantID: "tenant-1",
		Name:     "soc-charge",
		Commands: []appport.Command{
			{
				CUCode:   "cu-1",
				MetricID: contracts.MetricElectricalActivePowerSetpoint,
				Value:    model.FloatCommandValue(50),
			},
			{
				CUCode:   "cu-2",
				MetricID: contracts.MetricElectricalActivePowerSetpoint,
				Value:    model.FloatCommandValue(-50),
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if result.TaskID != "task-1" || result.Status != "running" {
		t.Fatalf("result = %+v", result)
	}

	req := srv.lastReq
	if req.GetTriggerType() != "automatic" || req.GetTaskType() != "control" {
		t.Fatalf("request = %+v", req)
	}
	if len(req.GetActions()) != 1 || req.GetActions()[0].GetExecutionPolicy() != "parallel" {
		t.Fatalf("actions = %+v", req.GetActions())
	}
	commands := req.GetActions()[0].GetCommands()
	if len(commands) != 2 {
		t.Fatalf("commands = %d", len(commands))
	}
	wantKey := string(contracts.MetricElectricalActivePowerSetpoint)
	if commands[0].GetCUCode() != "cu-1" || commands[0].GetPointKey() != wantKey || commands[0].GetFloatValue() != 50 {
		t.Fatalf("first command = %+v", commands[0])
	}
	if commands[1].GetPointKey() != wantKey || commands[1].GetFloatValue() != -50 {
		t.Fatalf("second command = %+v", commands[1])
	}
}

func TestClient_SubmitTask_SendsIdempotencyKey(t *testing.T) {
	t.Parallel()

	srv := &fakeDispatchServer{}
	dialer, cleanup := newBufconnDispatchServer(t, srv)
	defer cleanup()
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufdispatch-idem",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.SubmitTask(context.Background(), appport.Task{
		TenantID:       "tenant-1",
		Name:           "soc-charge",
		IdempotencyKey: "step-1:1",
		Commands: []appport.Command{{
			CUCode:   "cu-1",
			MetricID: contracts.MetricElectricalActivePowerSetpoint,
			Value:    model.FloatCommandValue(50),
		}},
	})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if srv.lastReq.GetIdempotencyKey() != "step-1:1" {
		t.Fatalf("idempotency key = %q", srv.lastReq.GetIdempotencyKey())
	}
}

func TestClient_SubmitTask_EmptyCommandsErrorsWithoutRPC(t *testing.T) {
	t.Parallel()

	srv := &fakeDispatchServer{}
	dialer, cleanup := newBufconnDispatchServer(t, srv)
	defer cleanup()
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufdispatch-empty",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.SubmitTask(context.Background(), appport.Task{TenantID: "tenant-1", Name: "empty"})
	if err == nil {
		t.Fatal("expected an error for empty commands")
	}
	if got := srv.calls.Load(); got != 0 {
		t.Fatalf("expected no RPC, got %d", got)
	}
}

func TestClient_SubmitTask_InvalidCommandValueErrorsWithoutRPC(t *testing.T) {
	t.Parallel()

	srv := &fakeDispatchServer{}
	dialer, cleanup := newBufconnDispatchServer(t, srv)
	defer cleanup()
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufdispatch-invalid",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.SubmitTask(context.Background(), appport.Task{
		TenantID: "tenant-1",
		Name:     "invalid",
		Commands: []appport.Command{{
			CUCode:   "cu-1",
			MetricID: contracts.MetricElectricalActivePowerSetpoint,
		}},
	})
	if err == nil {
		t.Fatal("expected an error for an unset command value")
	}
	if got := srv.calls.Load(); got != 0 {
		t.Fatalf("expected no RPC, got %d", got)
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv := &fakeDispatchServer{fail: true}
	dialer, cleanup := newBufconnDispatchServer(t, srv)
	defer cleanup()
	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-decision->dispatch",
		ConsecutiveFailures: 1,
		OpenTimeout:         time.Minute,
		IsSuccessful:        resilience.GRPCIsSuccessful,
	})
	client, err := NewClient(Config{
		Addr:        "passthrough:///bufdispatch-breaker",
		DialOptions: []grpc.DialOption{grpc.WithContextDialer(dialer)},
		Breaker:     breaker,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	task := appport.Task{
		TenantID: "tenant-1",
		Name:     "n",
		Commands: []appport.Command{{
			CUCode:   "cu-1",
			MetricID: contracts.MetricElectricalActivePowerSetpoint,
			Value:    model.FloatCommandValue(1),
		}},
	}
	if _, err := client.SubmitTask(context.Background(), task); err == nil {
		t.Fatal("expected the first call to surface the RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
	_, err = client.SubmitTask(context.Background(), task)
	if !errors.Is(err, resilience.ErrDependencyUnavailable) {
		t.Fatalf("err = %v, want ErrDependencyUnavailable", err)
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("breaker open still called dispatch, calls = %d", got)
	}
}

func TestNewClient_RequiresAddr(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("expected an error when Addr is empty")
	}
}
