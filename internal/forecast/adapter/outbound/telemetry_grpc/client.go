package telemetrygrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/forecast/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream telemetry gRPC
// service, mirroring internal/optimization/adapter/outbound/telemetry_grpc.Config.
type Config struct {
	Addr string // e.g. "127.0.0.1:5003"

	// DialOptions carries additional grpc.DialOption values appended after
	// the platform defaults. Production callers leave this nil; tests use
	// it to inject grpc.WithContextDialer for bufconn.
	DialOptions []grpc.DialOption

	// Timeout bounds outbound QueryAggregation calls that don't already
	// carry a context deadline. 0 disables (no timeout added).
	Timeout time.Duration
	// Breaker short-circuits QueryAggregation once too many failures
	// accumulate. nil disables (no circuit breaking).
	Breaker *gobreaker.CircuitBreaker[any]
}

// Client implements domain/port.TelemetryPort by calling
// TelemetryService.QueryAggregation.
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

// QueryAggregation forwards to TelemetryService.QueryAggregation requesting
// AVG (MovingAveragePredictor) and LAST (SamePeriodPriorPredictor) and
// converts the response into []port.AggregatedPoint.
func (c *Client) QueryAggregation(
	ctx context.Context, tenantID, cuCode, metricName string,
	start, end time.Time, stepSeconds int64,
) ([]port.AggregatedPoint, error) {
	resp, err := c.client.QueryAggregation(ctx, &telemetrypb.QueryAggregationRequest{
		TenantID:    tenantID,
		CUCode:      cuCode,
		MetricID:    metricName,
		StartTime:   timestamppb.New(start),
		EndTime:     timestamppb.New(end),
		StepSeconds: stepSeconds,
		Functions: []telemetrypb.AggFunction{
			telemetrypb.AggFunction_AGG_FUNCTION_AVG,
			telemetrypb.AggFunction_AGG_FUNCTION_LAST,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("telemetry_grpc: QueryAggregation: %w", err)
	}
	return aggregatedPointsFromProto(resp.GetPoints()), nil
}

func aggregatedPointsFromProto(in []*telemetrypb.AggregatedPoint) []port.AggregatedPoint {
	out := make([]port.AggregatedPoint, 0, len(in))
	for _, p := range in {
		if p == nil || p.GetWindowStart() == nil {
			continue
		}
		out = append(out, port.AggregatedPoint{
			Timestamp: p.GetWindowStart().AsTime().UTC(),
			Avg:       cloneFloat(p.Avg),
			Last:      cloneFloat(p.Last),
		})
	}
	return out
}

func cloneFloat(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}
