package gatewaygrpc

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gatewaypb "github.com/mushroomyuan/vpp-backend/api/gateway/proto/gen"
	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream gateway gRPC service.
type Config struct {
	Addr string // e.g. "127.0.0.1:5005"

	// DialOptions carries additional grpc.DialOption values appended after the
	// platform defaults (insecure creds, otel stats handler). Production callers
	// leave this nil; tests use it to inject grpc.WithContextDialer for bufconn.
	DialOptions []grpc.DialOption
}

// Client implements application/port.GatewayPort by calling GatewayService.ExecuteCommand.
type Client struct {
	client gatewaypb.GatewayServiceClient
	conn   *grpc.ClientConn
}

var _ appport.GatewayPort = (*Client)(nil)

// NewClient dials the gateway gRPC service. Caller must Close() on shutdown.
// The connection is instrumented with otelgrpc so outbound ExecuteCommand
// creates a client span and propagates trace context.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("gateway_grpc: addr is required")
	}
	conn, err := platformserver.DialGRPC(cfg.Addr, cfg.DialOptions...)
	if err != nil {
		return nil, fmt.Errorf("gateway_grpc: %w", err)
	}
	logrus.Infof("gateway_grpc: connected to %s (otel client enabled)", cfg.Addr)
	return &Client{
		client: gatewaypb.NewGatewayServiceClient(conn),
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

// ExecuteCommand forwards the ControlCommand to Gateway.
//
// Mapping:
//   - RPC success → GatewayAccepted (final outcome arrives via Kafka)
//   - gRPC business errors (NotFound, FailedPrecondition, InvalidArgument) → GatewayRejected
//   - transport / other errors → returned as error (application treats as rejection)
func (c *Client) ExecuteCommand(
	ctx context.Context,
	cmd *model.ControlCommand,
) (*appport.GatewayExecuteResult, error) {
	req, err := toProtoRequest(cmd)
	if err != nil {
		return &appport.GatewayExecuteResult{
			Status:  appport.GatewayRejected,
			Message: err.Error(),
		}, nil
	}

	_, err = c.client.ExecuteCommand(ctx, req)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.NotFound, codes.FailedPrecondition, codes.InvalidArgument, codes.AlreadyExists:
				return &appport.GatewayExecuteResult{
					Status:  appport.GatewayRejected,
					Message: st.Message(),
				}, nil
			}
		}
		return nil, fmt.Errorf("gateway ExecuteCommand: %w", err)
	}

	return &appport.GatewayExecuteResult{
		Status: appport.GatewayAccepted,
	}, nil
}

func toProtoRequest(cmd *model.ControlCommand) (*gatewaypb.ExecuteCommandRequest, error) {
	req := &gatewaypb.ExecuteCommandRequest{
		CommandID: cmd.ID,
		TenantID:  cmd.TenantID,
		CUCode:    cmd.CUCode,
		PointKey:  cmd.PointKey,
	}
	switch {
	case cmd.Value.BoolValue != nil:
		req.Value = &gatewaypb.ExecuteCommandRequest_BoolValue{BoolValue: *cmd.Value.BoolValue}
	case cmd.Value.IntValue != nil:
		req.Value = &gatewaypb.ExecuteCommandRequest_IntValue{IntValue: *cmd.Value.IntValue}
	case cmd.Value.FloatValue != nil:
		req.Value = &gatewaypb.ExecuteCommandRequest_FloatValue{FloatValue: *cmd.Value.FloatValue}
	case cmd.Value.StringValue != nil:
		req.Value = &gatewaypb.ExecuteCommandRequest_StringValue{StringValue: *cmd.Value.StringValue}
	default:
		return nil, fmt.Errorf("command value is unset")
	}
	return req, nil
}
