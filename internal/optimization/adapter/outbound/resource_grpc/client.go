// Package resourcegrpc implements domain/port.ResourcePort by calling
// ResourceService.GetAsset directly — internal direct connection, same
// trust model as gateway->telemetry (discussion/2026-09-04.md §3.1),
// same "只读直连 resource" precedent as internal/simulator's resource
// client (internal/simulator/client/resource/client.go).
package resourcegrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/optimization/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream resource gRPC
// service, mirroring telemetry_grpc.Config.
type Config struct {
	Addr        string
	DialOptions []grpc.DialOption
	Timeout     time.Duration
	Breaker     *gobreaker.CircuitBreaker[any]
}

// Client implements domain/port.ResourcePort by calling
// ResourceService.GetAsset once per scope entry.
type Client struct {
	client resourcepb.ResourceServiceClient
	conn   *grpc.ClientConn
}

var _ port.ResourcePort = (*Client)(nil)

// NewClient dials the resource gRPC service. Caller must Close() on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("resource_grpc: addr is required")
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
		return nil, fmt.Errorf("resource_grpc: %w", err)
	}
	logrus.Infof("resource_grpc: connected to %s (otel client enabled)", cfg.Addr)
	return &Client{
		client: resourcepb.NewResourceServiceClient(conn),
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

// GetCapacityKW resolves each scope entry's RatedCapacityKW via
// GetAsset — Resource's static config, not the AssetRuntime cache (see
// domain/port.ResourcePort's doc comment for why).
//
// v1 simplification: every scope entry is assumed to be an AssetID, since
// that is the level RatedCapacityKW is actually defined at (CU has no
// capacity field of its own — see internal/resource/domain/model/cu.go).
// This branch has no real caller yet (AggregateTarget, see
// domain/model/target.go); if Market ends up needing CUCode-scoped
// AggregateTargets, resolving CU -> ParentID (Asset) first belongs here,
// not before there's an actual caller to drive the requirement.
//
// A NotFound scope entry is omitted from the result (not an error) so
// splitByCapacity can distinguish "unknown capacity" from "zero-rated" —
// see domain/service/allocate.go.
func (c *Client) GetCapacityKW(ctx context.Context, tenantID string, scope []string) (map[string]float64, error) {
	out := make(map[string]float64, len(scope))
	for _, id := range scope {
		resp, err := c.client.GetAsset(ctx, &resourcepb.GetAssetRequest{
			TenantID: tenantID,
			ID:       id,
		})
		if err != nil {
			if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
				continue
			}
			return nil, fmt.Errorf("resource_grpc: GetAsset %s: %w", id, err)
		}
		out[id] = resp.GetRatedCapacityKW()
	}
	return out, nil
}
