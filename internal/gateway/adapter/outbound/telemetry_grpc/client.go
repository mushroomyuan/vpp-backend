package telemetrygrpc

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	telemetrypb "github.com/mushroomyuan/vpp-backend/api/telemetry/proto/gen"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
)

// Config holds the connection parameters for the upstream telemetry gRPC service.
type Config struct {
	Addr string // e.g. "127.0.0.1:5003"
}

// TelemetryGRPCClient implements port.TelemetryClient by forwarding to the
// vpp-telemetry service via TelemetryService.IngestTelemetry.
type TelemetryGRPCClient struct {
	client telemetrypb.TelemetryServiceClient
	conn   *grpc.ClientConn
}

var _ port.TelemetryClient = (*TelemetryGRPCClient)(nil)

// NewTelemetryGRPCClient dials the telemetry gRPC service and returns a client.
// The caller must close the connection on shutdown by calling Close().
func NewTelemetryGRPCClient(cfg Config) (*TelemetryGRPCClient, error) {
	if cfg.Addr == "" {
		return nil, fmt.Errorf("telemetry_grpc: addr is required")
	}
	conn, err := grpc.NewClient(cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("telemetry_grpc: dial %s: %w", cfg.Addr, err)
	}
	logrus.Infof("telemetry_grpc: connected to %s", cfg.Addr)
	return &TelemetryGRPCClient{
		client: telemetrypb.NewTelemetryServiceClient(conn),
		conn:   conn,
	}, nil
}

// Close releases the underlying gRPC connection. Called during server shutdown.
func (c *TelemetryGRPCClient) Close() error {
	return c.conn.Close()
}

// Ingest forwards the StandardTelemetry to TelemetryService.IngestTelemetry.
// One call = one gRPC request = one CU push at one timestamp.
func (c *TelemetryGRPCClient) Ingest(ctx context.Context, t *model.StandardTelemetry) error {
	req := &telemetrypb.IngestTelemetryRequest{
		TenantID:  t.TenantID,
		CUCode:    t.CUCode,
		Timestamp: timestamppb.New(t.Timestamp),
		Metrics:   mapMetrics(t.Metrics),
	}
	_, err := c.client.IngestTelemetry(ctx, req)
	if err != nil {
		return fmt.Errorf("IngestTelemetry rpc: %w", err)
	}
	return nil
}

// mapMetrics converts gateway domain MetricValues to the telemetry proto wire format.
func mapMetrics(in []model.MetricValue) []*telemetrypb.MetricValue {
	out := make([]*telemetrypb.MetricValue, 0, len(in))
	for _, m := range in {
		out = append(out, &telemetrypb.MetricValue{
			Name:    m.Name,
			Value:   m.Value,
			Type:    mapMetricType(m.Type),
			Quality: mapQuality(m.Quality),
		})
	}
	return out
}

func mapMetricType(t model.MetricType) telemetrypb.MetricType {
	switch t {
	case model.MetricTypeDiscrete:
		return telemetrypb.MetricType_METRIC_TYPE_DISCRETE
	default:
		return telemetrypb.MetricType_METRIC_TYPE_ANALOG
	}
}

func mapQuality(q model.QualityStatus) telemetrypb.QualityStatus {
	switch q {
	case model.QualityBad:
		return telemetrypb.QualityStatus_QUALITY_STATUS_BAD
	case model.QualityUncertain:
		return telemetrypb.QualityStatus_QUALITY_STATUS_UNCERTAIN
	default:
		return telemetrypb.QualityStatus_QUALITY_STATUS_GOOD
	}
}
