package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gatewaycommand "github.com/mushroomyuan/vpp-backend/gateway/application/command"
	gatewaymodel "github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	telemetryquery "github.com/mushroomyuan/vpp-backend/telemetry/application/query"
)

// TestReceiveTelemetry_IngestsAndSnapshots exercises the full telemetry
// ingestion chain: gateway.ReceiveTelemetry -> mapping lookup (ExternalID ->
// CUCode) -> a real gRPC call over bufconn to telemetry.IngestTelemetry ->
// TimescaleDB batch write + Redis snapshot cache update.
//
// The HTTP boundary that normally fronts ReceiveTelemetry is bypassed, the
// same way the dispatch tests bypass the gRPC/HTTP boundary that normally
// fronts SubmitTask — the goal is to exercise the real cross-service
// application/adapter wiring, not the transport framing.
func TestReceiveTelemetry_IngestsAndSnapshots(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-telemetry-ingest"
	const cuCode = "cu-telemetry-001"

	_, err := e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID:       tenantID,
		ExternalSystem: "ems-test",
		ExternalID:     "device-telemetry-1",
		CUCode:         cuCode,
	})
	require.NoError(t, err, "seed active device mapping")

	ts := time.Now().UTC().Truncate(time.Millisecond)
	_, err = e.Gateway.Commands.ReceiveTelemetry.Handle(ctx, gatewaycommand.ReceiveTelemetry{
		Telemetry: &gatewaymodel.ExternalTelemetry{
			TenantID:       tenantID,
			ExternalSystem: "ems-test",
			ExternalID:     "device-telemetry-1",
			Timestamp:      ts,
			Metrics: []gatewaymodel.ExternalMetric{
				{Name: "active_power", Value: 123.45},
				{Name: "voltage", Value: 220.1},
			},
		},
	})
	require.NoError(t, err, "ReceiveTelemetry")

	var snapshot *telemetryquery.SnapshotView
	requireEventuallyf(t, func() bool {
		res, qErr := e.Telemetry.Queries.GetSnapshot.Handle(ctx, telemetryquery.GetSnapshot{
			TenantID: tenantID,
			CUCode:   cuCode,
		})
		if qErr != nil {
			return false
		}
		snapshot = res
		return len(snapshot.Metrics) > 0
	}, "snapshot for %s/%s never appeared in Redis", tenantID, cuCode)

	require.Equal(t, tenantID, snapshot.TenantID)
	require.Equal(t, cuCode, snapshot.CUCode)
	require.InDelta(t, 123.45, snapshot.Metrics["active_power"], 0.001)
	require.InDelta(t, 220.1, snapshot.Metrics["voltage"], 0.001)
	require.False(t, snapshot.Stale)

	// Also verify the raw TimescaleDB write via QueryTelemetry, confirming
	// the chain landed real rows and not just the Redis cache.
	records, err := e.Telemetry.Queries.QueryTelemetry.Handle(ctx, telemetryquery.QueryTelemetry{
		TenantID:   tenantID,
		CUCode:     cuCode,
		MetricName: "active_power",
		StartTime:  ts.Add(-time.Minute),
		EndTime:    ts.Add(time.Minute),
	})
	require.NoError(t, err, "QueryTelemetry")
	require.NotEmpty(t, records, "expected at least one persisted telemetry_records row")
}
