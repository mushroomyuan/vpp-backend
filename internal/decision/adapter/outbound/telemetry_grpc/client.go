// Package telemetrygrpc implements port.TelemetryPort with TelemetryService.GetSnapshots.
package telemetrygrpc

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/resilience"
	platformserver "github.com/mushroomyuan/vpp-backend/platform/server"
)

// Config holds the connection parameters for the upstream telemetry service.
type Config struct {
	Addr string

	// DialOptions are appended after the platform defaults. Tests inject bufconn here.
	DialOptions []grpc.DialOption

	// Timeout bounds calls that do not already carry a deadline. 0 disables it.
	Timeout time.Duration
	// Breaker short-circuits calls after repeated failures. nil disables it.
	Breaker *gobreaker.CircuitBreaker[any]
}

// Client calls TelemetryService.GetSnapshots.
type Client struct {
	client telemetrypb.TelemetryServiceClient
	conn   *grpc.ClientConn
}

var _ port.TelemetryPort = (*Client)(nil)

// NewClient dials telemetry. The caller must Close on shutdown.
func NewClient(cfg Config) (*Client, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("telemetry_grpc: addr is required")
	}
	conn, err := dial(cfg.Addr, cfg.DialOptions, cfg.Timeout, cfg.Breaker)
	if err != nil {
		return nil, fmt.Errorf("telemetry_grpc: %w", err)
	}
	logrus.Infof("telemetry_grpc: dialed %s", cfg.Addr)
	return &Client{client: telemetrypb.NewTelemetryServiceClient(conn), conn: conn}, nil
}

// Close releases the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// GetSnapshots reads per-metric value, observation time, and quality for the named CUs.
func (c *Client) GetSnapshots(ctx context.Context, query port.SnapshotQuery) ([]port.CUSnapshot, error) {
	if query.TenantID == "" {
		return nil, fmt.Errorf("telemetry_grpc: tenant id is required")
	}
	if len(query.CUCodes) == 0 {
		return nil, fmt.Errorf("telemetry_grpc: at least one CU code is required")
	}
	if len(query.MetricIDs) == 0 {
		return nil, fmt.Errorf("telemetry_grpc: at least one metric id is required")
	}
	if query.StaleAge < 0 {
		return nil, fmt.Errorf("telemetry_grpc: stale age must not be negative")
	}

	metricIDs := make([]string, len(query.MetricIDs))
	for i, id := range query.MetricIDs {
		metricIDs[i] = string(id)
	}
	resp, err := c.client.GetSnapshots(ctx, &telemetrypb.GetSnapshotsRequest{
		TenantID:        query.TenantID,
		CUCodes:         append([]string(nil), query.CUCodes...),
		MetricIDs:       metricIDs,
		StaleAgeSeconds: int64(query.StaleAge / time.Second),
	})
	if err != nil {
		return nil, fmt.Errorf("telemetry_grpc: GetSnapshots: %w", err)
	}

	out := make([]port.CUSnapshot, 0, len(resp.GetSnapshots()))
	for _, snap := range resp.GetSnapshots() {
		if snap == nil {
			continue
		}
		samples := make([]port.MetricSample, 0, len(snap.GetMetrics()))
		for _, state := range snap.GetMetrics() {
			if state == nil {
				continue
			}
			var observed time.Time
			if ts := state.GetObservedAt(); ts != nil {
				observed = ts.AsTime()
			}
			samples = append(samples, port.MetricSample{
				MetricID:   contracts.MetricID(state.GetMetricID()),
				Value:      state.GetDoubleValue(),
				ObservedAt: observed,
				Quality:    qualityFromProto(state.GetQuality()),
			})
		}
		var updated time.Time
		if ts := snap.GetUpdatedAt(); ts != nil {
			updated = ts.AsTime()
		}
		out = append(out, port.CUSnapshot{
			CUCode:    snap.GetCUCode(),
			Metrics:   samples,
			UpdatedAt: updated,
			Stale:     snap.GetStale(),
		})
	}
	return out, nil
}

func qualityFromProto(q telemetrypb.QualityStatus) port.Quality {
	switch q {
	case telemetrypb.QualityStatus_QUALITY_STATUS_GOOD:
		return port.QualityGood
	case telemetrypb.QualityStatus_QUALITY_STATUS_BAD:
		return port.QualityBad
	case telemetrypb.QualityStatus_QUALITY_STATUS_UNCERTAIN:
		return port.QualityUncertain
	default:
		return port.QualityUnspecified
	}
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
