// Package resourcegrpc loads CU point bindings from ResourceService.ListPoints.
package resourcegrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"

	resourcepb "github.com/mushroomyuan/vpp-backend/api/resource/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/binding"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

const (
	pageSize = 200
	maxPages = 50
)

// Config holds the connection parameters for the upstream resource service.
type Config struct {
	Addr        string
	DialOptions []grpc.DialOption
	Timeout     time.Duration
	Breaker     *gobreaker.CircuitBreaker[any]
}

// Client lists point bindings. It does not resolve scopes or read runtime values.
type Client struct {
	client resourcepb.ResourceServiceClient
	conn   *grpc.ClientConn
}

var _ binding.Catalog = (*Client)(nil)

// NewClient dials resource. The caller must Close on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("resource_grpc: addr is required")
	}
	conn, err := dial(cfg.Addr, cfg.DialOptions, cfg.Timeout, cfg.Breaker)
	if err != nil {
		return nil, fmt.Errorf("resource_grpc: %w", err)
	}
	logrus.Infof("resource_grpc: dialed %s", cfg.Addr)
	return &Client{client: resourcepb.NewResourceServiceClient(conn), conn: conn}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// ListCUBindings pages ListPoints for one CU. Disabled points are included.
// The caller caches the result. This method does not keep one.
func (c *Client) ListCUBindings(ctx context.Context, tenantID, cuID string) ([]binding.Binding, error) {
	if tenantID == "" || cuID == "" {
		return nil, fmt.Errorf("resource_grpc: tenant and cu are required")
	}
	var all []binding.Binding
	for page := 0; page < maxPages; page++ {
		resp, err := c.client.ListPoints(ctx, &resourcepb.ListPointsRequest{
			TenantID: tenantID,
			CUID:     cuID,
			Offset:   int32(page * pageSize),
			Limit:    pageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("resource_grpc: list points: %w", err)
		}
		points := resp.GetPoints()
		for _, point := range points {
			mapped, err := bindingFromProto(point)
			if err != nil {
				return nil, err
			}
			all = append(all, mapped)
		}
		if len(points) < pageSize {
			return all, nil
		}
	}
	return nil, fmt.Errorf("resource_grpc: cu %s has more than %d bindings", cuID, pageSize*maxPages)
}

func bindingFromProto(point *resourcepb.Point) (binding.Binding, error) {
	if point == nil {
		return binding.Binding{}, fmt.Errorf("resource_grpc: point is nil")
	}
	mode, err := accessFromProto(point.GetAccessMode())
	if err != nil {
		return binding.Binding{}, err
	}
	if point.GetID() == "" || point.GetMetricID() == "" || point.GetExternalAddress() == "" {
		return binding.Binding{}, fmt.Errorf("resource_grpc: point %s is missing id, metric, or external address", point.GetID())
	}
	if point.GetRevision() <= 0 {
		return binding.Binding{}, fmt.Errorf("resource_grpc: point %s has no revision", point.GetID())
	}
	return binding.Binding{
		PointID:         point.GetID(),
		MetricID:        point.GetMetricID(),
		ExternalAddress: point.GetExternalAddress(),
		AccessMode:      mode,
		Scale:           point.GetScale(),
		Offset:          point.GetOffset(),
		Enabled:         point.GetEnabled(),
		Revision:        point.GetRevision(),
		Safety:          safetyFromProto(point.GetSafetyConstraint()),
	}, nil
}

func accessFromProto(mode resourcepb.PointAccessMode) (binding.AccessMode, error) {
	switch mode {
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ:
		return binding.AccessRead, nil
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_WRITE:
		return binding.AccessWrite, nil
	case resourcepb.PointAccessMode_POINT_ACCESS_MODE_READ_WRITE:
		return binding.AccessReadWrite, nil
	default:
		return "", fmt.Errorf("resource_grpc: access mode %s is not usable", mode.String())
	}
}

func safetyFromProto(in *resourcepb.PointSafetyConstraint) *binding.Safety {
	if in == nil {
		return nil
	}
	out := &binding.Safety{Version: in.GetVersion()}
	if min := in.GetMinValue(); min != nil {
		v := min.GetValue()
		out.MinValue = &v
	}
	if max := in.GetMaxValue(); max != nil {
		v := max.GetValue()
		out.MaxValue = &v
	}
	if rate := in.GetMaxChangePerSecond(); rate != nil {
		v := rate.GetValue()
		out.MaxChangePerSecond = &v
	}
	return out
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
