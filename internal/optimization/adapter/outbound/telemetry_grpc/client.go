// Package telemetrygrpc implements domain/port.TelemetryPort by calling
// TelemetryService.GetSnapshot directly (internal direct connection, not
// through APISIX — see discussion/2026-09-04.md §3.1: this matches the
// existing gateway->telemetry and dispatch->gateway trust model).
package telemetrygrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream telemetry gRPC
// service, mirroring internal/dispatch/adapter/outbound/gateway_grpc.Config.
type Config struct {
	Addr string // e.g. "127.0.0.1:5003"

	// DialOptions carries additional grpc.DialOption values appended after
	// the platform defaults. Production callers leave this nil; tests use
	// it to inject grpc.WithContextDialer for bufconn.
	DialOptions []grpc.DialOption

	// Timeout bounds outbound GetSnapshot calls that don't already carry a
	// context deadline. 0 disables (no timeout added).
	Timeout time.Duration
	// Breaker short-circuits GetSnapshot once too many failures
	// accumulate. nil disables (no circuit breaking). Optimization is the
	// first high-frequency caller of Telemetry — see the Optimization
	// design plan §9 for why this defaults to enabled at the config layer
	// (a later step), unlike dispatch->gateway's default-disabled breaker.
	Breaker *gobreaker.CircuitBreaker[any]
}

// Client implements domain/port.TelemetryPort by calling
// TelemetryService.GetSnapshot.
type Client struct {
	client telemetrypb.TelemetryServiceClient
	conn   *grpc.ClientConn
}

var _ port.TelemetryPort = (*Client)(nil)

// NewClient dials the telemetry gRPC service. Caller must Close() on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("telemetry_grpc: addr is required")
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
		return nil, fmt.Errorf("telemetry_grpc: %w", err)
	}
	logrus.Infof("telemetry_grpc: connected to %s (otel client enabled)", cfg.Addr)
	return &Client{
		client: telemetrypb.NewTelemetryServiceClient(conn),
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

// GetSnapshot forwards to TelemetryService.GetSnapshot and converts the
// response into domain/port.Snapshot. It does not use StaleAgeSeconds'
// server-side override — Optimization uses the server default (5 min),
// matching how the decision loop's own cooldown/interval, not a per-call
// override, is what should tune "how fresh is fresh enough" (a later
// step can revisit if this proves too coarse).
func (c *Client) GetSnapshot(ctx context.Context, tenantID, cuCode string) (port.Snapshot, error) {
	resp, err := c.client.GetSnapshot(ctx, &telemetrypb.GetSnapshotRequest{
		TenantID: tenantID,
		CUCode:   cuCode,
	})
	if err != nil {
		return port.Snapshot{}, fmt.Errorf("telemetry_grpc: GetSnapshot: %w", err)
	}
	return port.Snapshot{
		CUCode:  resp.GetCUCode(),
		Metrics: resp.GetMetrics(),
		Stale:   resp.GetStale(),
	}, nil
}
