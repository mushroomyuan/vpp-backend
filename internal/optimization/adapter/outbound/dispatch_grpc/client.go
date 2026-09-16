// Package dispatchgrpc implements application/port.DispatchPort by
// calling DispatchService.SubmitTask directly — internal direct
// connection, same trust model as gateway->telemetry and
// dispatch->gateway (discussion/2026-09-04.md §3.1's "已决定" for
// Optimization->Dispatch specifically).
package dispatchgrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	appport "github.com/mushroomyuan/vpp-backend/optimization/application/port"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream dispatch gRPC
// service, mirroring telemetry_grpc.Config / resource_grpc.Config.
type Config struct {
	Addr        string
	DialOptions []grpc.DialOption
	Timeout     time.Duration
	Breaker     *gobreaker.CircuitBreaker[any]
}

// Client implements application/port.DispatchPort by calling
// DispatchService.SubmitTask.
type Client struct {
	client dispatchpb.DispatchServiceClient
	conn   *grpc.ClientConn
}

var _ appport.DispatchPort = (*Client)(nil)

// NewClient dials the dispatch gRPC service. Caller must Close() on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("dispatch_grpc: addr is required")
	}
	var interceptors []grpc.UnaryClientInterceptor
	if cfg.Timeout > 0 {
		interceptors = append(interceptors, resilience.UnaryClientTimeoutInterceptor(cfg.Timeout))
	}
	if cfg.Breaker != nil {
		interceptors = append(interceptors, resilience.UnaryClientBreakerInterceptor(cfg.Breaker))
	}
	dialOpts := cfg.DialOptions
	if len(interceptors) > 0 {
		dialOpts = append([]grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}, dialOpts...)
	}
	conn, err := platformserver.DialGRPC(cfg.Addr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dispatch_grpc: %w", err)
	}
	logrus.Infof("dispatch_grpc: connected to %s (otel client enabled)", cfg.Addr)
	return &Client{
		client: dispatchpb.NewDispatchServiceClient(conn),
		conn:   conn,
	}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// SubmitTask packs commands into a single dispatch Task with one
// parallel ActionSpec — Optimization's commands from one decision cycle
// target independent CUs (see domain/service/allocate.go: each
// CommandSpec is already resolved for one CU/Point) with no sequencing
// requirement between them, so "parallel" (not "sequential") is the
// right ExecutionPolicy, matching discussion §3.4's "多 CU 同时调节通常
// ExecutionPolicy=parallel".
//
// TriggerType is always "automatic" — see application/port.DispatchPort's
// doc comment for why this reuses dispatch's existing TriggerType field.
func (c *Client) SubmitTask(
	ctx context.Context, tenantID, name string, commands []model.CommandSpec,
) (appport.SubmitResult, error) {
	if len(commands) == 0 {
		return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: SubmitTask requires at least one command")
	}

	specs := make([]*dispatchpb.CommandSpec, 0, len(commands))
	for _, cmd := range commands {
		spec, err := toProtoCommandSpec(cmd)
		if err != nil {
			return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: %w", err)
		}
		specs = append(specs, spec)
	}

	resp, err := c.client.SubmitTask(ctx, &dispatchpb.SubmitTaskRequest{
		TenantID:    tenantID,
		Name:        name,
		TaskType:    "control",
		TriggerType: "automatic",
		Actions: []*dispatchpb.ActionSpec{
			{
				Name:            name,
				ActionType:      "control",
				Sequence:        1,
				ExecutionPolicy: "parallel",
				Commands:        specs,
			},
		},
	})
	if err != nil {
		return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: SubmitTask: %w", err)
	}

	return appport.SubmitResult{
		TaskID: resp.GetTaskID(),
		Status: resp.GetStatus(),
	}, nil
}

func toProtoCommandSpec(cmd model.CommandSpec) (*dispatchpb.CommandSpec, error) {
	if err := cmd.Value.Validate(); err != nil {
		return nil, fmt.Errorf("command for CU %s point %s: %w", cmd.CUCode, cmd.PointKey, err)
	}
	spec := &dispatchpb.CommandSpec{
		CUCode:         cmd.CUCode,
		PointKey:       cmd.PointKey,
		TimeoutSeconds: cmd.TimeoutSeconds,
		MaxRetries:     cmd.MaxRetries,
	}
	switch {
	case cmd.Value.BoolValue != nil:
		spec.Value = &dispatchpb.CommandSpec_BoolValue{BoolValue: *cmd.Value.BoolValue}
	case cmd.Value.IntValue != nil:
		spec.Value = &dispatchpb.CommandSpec_IntValue{IntValue: *cmd.Value.IntValue}
	case cmd.Value.FloatValue != nil:
		spec.Value = &dispatchpb.CommandSpec_FloatValue{FloatValue: *cmd.Value.FloatValue}
	case cmd.Value.StringValue != nil:
		spec.Value = &dispatchpb.CommandSpec_StringValue{StringValue: *cmd.Value.StringValue}
	}
	return spec, nil
}
