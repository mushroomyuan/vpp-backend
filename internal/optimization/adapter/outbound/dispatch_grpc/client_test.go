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

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// fakeDispatchServer implements dispatchpb.DispatchServiceServer, capturing
// the last SubmitTaskRequest it received.
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
	cleanup := func() { grpcSrv.Stop() }
	return dialer, cleanup
}

func TestClient_SubmitTask_BuildsExpectedRequest(t *testing.T) {
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

	commands := []model.CommandSpec{
		{CUCode: "cu-1", PointKey: "active_power_setpoint_kw", Value: model.FloatCommandValue(50)},
		{CUCode: "cu-2", PointKey: "active_power_setpoint_kw", Value: model.FloatCommandValue(-50)},
	}

	result, err := client.SubmitTask(context.Background(), "tenant-1", "soc_threshold_cycle", commands)
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	if result.TaskID != "task-1" || result.Status != "running" {
		t.Errorf("unexpected result: %+v", result)
	}

	req := srv.lastReq
	if req.GetTriggerType() != "automatic" {
		t.Errorf("TriggerType = %q, want %q", req.GetTriggerType(), "automatic")
	}
	if req.GetTaskType() != "control" {
		t.Errorf("TaskType = %q, want %q", req.GetTaskType(), "control")
	}
	if len(req.GetActions()) != 1 {
		t.Fatalf("expected exactly 1 ActionSpec, got %d", len(req.GetActions()))
	}
	action := req.GetActions()[0]
	if action.GetExecutionPolicy() != "parallel" {
		t.Errorf("ExecutionPolicy = %q, want %q", action.GetExecutionPolicy(), "parallel")
	}
	if len(action.GetCommands()) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(action.GetCommands()))
	}
	if action.GetCommands()[0].GetCUCode() != "cu-1" || action.GetCommands()[0].GetFloatValue() != 50 {
		t.Errorf("unexpected first command: %+v", action.GetCommands()[0])
	}
	if action.GetCommands()[1].GetCUCode() != "cu-2" || action.GetCommands()[1].GetFloatValue() != -50 {
		t.Errorf("unexpected second command: %+v", action.GetCommands()[1])
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

	_, err = client.SubmitTask(context.Background(), "tenant-1", "empty", nil)
	if err == nil {
		t.Fatal("expected an error for empty commands")
	}
	if got := srv.calls.Load(); got != 0 {
		t.Fatalf("expected no RPC to be made for empty commands, got %d calls", got)
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

	_, err = client.SubmitTask(context.Background(), "tenant-1", "invalid", []model.CommandSpec{
		{CUCode: "cu-1", PointKey: "p"}, // no value set
	})
	if err == nil {
		t.Fatal("expected an error for an unset CommandValue")
	}
	if got := srv.calls.Load(); got != 0 {
		t.Fatalf("expected no RPC to be made for an invalid command, got %d calls", got)
	}
}

func TestClient_BreakerOpen_ReturnsErrDependencyUnavailable(t *testing.T) {
	t.Parallel()

	srv := &fakeDispatchServer{fail: true}
	dialer, cleanup := newBufconnDispatchServer(t, srv)
	defer cleanup()

	breaker := resilience.NewBreaker[any](resilience.BreakerConfig{
		Enabled:             true,
		Name:                "test-optimization->dispatch",
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

	commands := []model.CommandSpec{{CUCode: "cu-1", PointKey: "p", Value: model.FloatCommandValue(1)}}
	ctx := context.Background()

	if _, err := client.SubmitTask(ctx, "tenant-1", "n", commands); err == nil {
		t.Fatal("expected the first call to surface the underlying RPC failure")
	}
	if got := srv.calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 real RPC after the first call, got %d", got)
	}

	_, err = client.SubmitTask(ctx, "tenant-1", "n", commands)
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
