// Package dispatchgrpc implements port.DispatchPort with DispatchService.SubmitTask.
// MetricID is written into the proto PointKey field. Dispatch still treats that string as opaque.
package dispatchgrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"

	dispatchpb "github.com/mushroomyuan/vpp-backend/api/dispatch/proto/gen"
	appport "github.com/mushroomyuan/vpp-backend/decision/application/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream dispatch service.
type Config struct {
	Addr        string
	DialOptions []grpc.DialOption
	Timeout     time.Duration
	Breaker     *gobreaker.CircuitBreaker[any]
}

// Client calls DispatchService.SubmitTask.
type Client struct {
	client dispatchpb.DispatchServiceClient
	conn   *grpc.ClientConn
}

var _ appport.DispatchPort = (*Client)(nil)

// NewClient dials dispatch. The caller must Close on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("dispatch_grpc: addr is required")
	}
	conn, err := dial(cfg.Addr, cfg.DialOptions, cfg.Timeout, cfg.Breaker)
	if err != nil {
		return nil, fmt.Errorf("dispatch_grpc: %w", err)
	}
	logrus.Infof("dispatch_grpc: dialed %s", cfg.Addr)
	return &Client{client: dispatchpb.NewDispatchServiceClient(conn), conn: conn}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// SubmitTask sends one control task whose commands run in parallel.
// TriggerType is automatic. IdempotencyKey is forwarded unchanged.
func (c *Client) SubmitTask(ctx context.Context, task appport.Task) (appport.SubmitResult, error) {
	if task.TenantID == "" {
		return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: tenant id is required")
	}
	if task.Name == "" {
		return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: task name is required")
	}
	if len(task.Commands) == 0 {
		return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: SubmitTask requires at least one command")
	}

	specs := make([]*dispatchpb.CommandSpec, 0, len(task.Commands))
	for _, cmd := range task.Commands {
		spec, err := toProtoCommand(cmd)
		if err != nil {
			return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: %w", err)
		}
		specs = append(specs, spec)
	}

	resp, err := c.client.SubmitTask(ctx, &dispatchpb.SubmitTaskRequest{
		TenantID:       task.TenantID,
		Name:           task.Name,
		TaskType:       "control",
		TriggerType:    "automatic",
		IdempotencyKey: task.IdempotencyKey,
		Actions: []*dispatchpb.ActionSpec{{
			Name:            task.Name,
			ActionType:      "control",
			Sequence:        1,
			ExecutionPolicy: "parallel",
			Commands:        specs,
		}},
	})
	if err != nil {
		return appport.SubmitResult{}, fmt.Errorf("dispatch_grpc: SubmitTask: %w", err)
	}
	return appport.SubmitResult{TaskID: resp.GetTaskID(), Status: resp.GetStatus()}, nil
}

func toProtoCommand(cmd appport.Command) (*dispatchpb.CommandSpec, error) {
	if cmd.CUCode == "" {
		return nil, fmt.Errorf("command: cu code is required")
	}
	if cmd.MetricID == "" {
		return nil, fmt.Errorf("command: metric id is required")
	}
	if err := cmd.Value.Validate(); err != nil {
		return nil, fmt.Errorf("command for CU %s metric %s: %w", cmd.CUCode, cmd.MetricID, err)
	}
	spec := &dispatchpb.CommandSpec{
		CUCode:         cmd.CUCode,
		PointKey:       string(cmd.MetricID),
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

func dial(addr string, extra []grpc.DialOption, timeout time.Duration, breaker *gobreaker.CircuitBreaker[any]) (*grpc.ClientConn, error) {
	var interceptors []grpc.UnaryClientInterceptor
	if timeout > 0 {
		interceptors = append(interceptors, resilience.UnaryClientTimeoutInterceptor(timeout))
	}
	if breaker != nil {
		interceptors = append(interceptors, resilience.UnaryClientBreakerInterceptor(breaker))
	}
	opts := extra
	if len(interceptors) > 0 {
		opts = append([]grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}, extra...)
	}
	return platformserver.DialGRPC(addr, opts...)
}
