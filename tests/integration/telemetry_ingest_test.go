package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	gatewaycommand "github.com/mushroomyuan/vpp-backend/gateway/application/command"
	gatewaymodel "github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	resourcecommand "github.com/mushroomyuan/vpp-backend/resource/application/command"
	resourcemodel "github.com/mushroomyuan/vpp-backend/resource/domain/model"
	telemetryquery "github.com/mushroomyuan/vpp-backend/telemetry/application/query"
)

// TestReceiveTelemetry_IngestsAndSnapshots exercises the full telemetry
// ingestion chain: gateway.ReceiveTelemetry -> mapping lookup (ExternalID ->
// CUCode) -> Resource point bindings -> canonical conversion -> a real gRPC
// call over bufconn to telemetry.IngestTelemetry -> TimescaleDB batch write +
// Redis snapshot cache update.
//
// The HTTP boundary that normally fronts ReceiveTelemetry is bypassed, the
// same way the dispatch tests bypass the gRPC/HTTP boundary that normally
// fronts SubmitTask — the goal is to exercise the real cross-service
// application/adapter wiring, not the transport framing.
func TestReceiveTelemetry_IngestsAndSnapshots(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()

	const tenantID = "tenant-telemetry-ingest"
	site, err := e.Resource.Commands.CreateSite.Handle(ctx, resourcecommand.CreateSite{
		TenantID: tenantID,
		Name:     "ingest-site",
	})
	require.NoError(t, err)
	assetID := mustAsset(t, ctx, e, tenantID, site.SiteID, "ingest-asset")
	cuCode := mustCU(t, ctx, e, tenantID, assetID, "ingest-cu", "battery")
	_, err = e.Resource.Commands.CreatePoint.Handle(ctx, resourcecommand.CreatePoint{
		TenantID:        tenantID,
		AssetID:         assetID,
		CUID:            cuCode,
		MetricID:        string(contracts.MetricElectricalActivePower),
		ExternalAddress: "vendor.active_power",
		AccessMode:      resourcemodel.AccessModeRead,
		Scale:           0.001,
		Enabled:         true,
	})
	require.NoError(t, err)
	_, err = e.Resource.Commands.CreatePoint.Handle(ctx, resourcecommand.CreatePoint{
		TenantID:        tenantID,
		AssetID:         assetID,
		CUID:            cuCode,
		MetricID:        string(contracts.MetricEnergyStorageStateOfCharge),
		ExternalAddress: "vendor.soc",
		AccessMode:      resourcemodel.AccessModeRead,
		Scale:           1,
		Offset:          -5,
		Enabled:         true,
	})
	require.NoError(t, err)

	_, err = e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID:       tenantID,
		ExternalSystem: "ems-test",
		ExternalID:     "device-telemetry-1",
		CUCode:         cuCode,
	})
	require.NoError(t, err, "seed active device mapping")

	ts := time.Now().UTC().Truncate(time.Millisecond)
	result, err := e.Gateway.Commands.ReceiveTelemetry.Handle(ctx, gatewaycommand.ReceiveTelemetry{
		Telemetry: &gatewaymodel.ExternalTelemetry{
			TenantID:       tenantID,
			ExternalSystem: "ems-test",
			ExternalID:     "device-telemetry-1",
			Timestamp:      ts,
			Metrics: []gatewaymodel.ExternalMetric{
				{ExternalAddress: "vendor.active_power", Value: 123450},
				{ExternalAddress: "vendor.soc", Value: 60.5},
				{ExternalAddress: "vendor.unknown", Value: 9},
			},
		},
	})
	require.NoError(t, err, "ReceiveTelemetry")
	require.Equal(t, 2, result.Accepted)
	require.Equal(t, 1, result.Isolated)

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
	require.InDelta(t, 123.45, metricValue(snapshot, "electrical.active_power.v1"), 0.001)
	require.InDelta(t, 55.5, metricValue(snapshot, "energy_storage.state_of_charge.v1"), 0.001)
	require.Zero(t, metricValue(snapshot, "vendor.unknown"))
	require.Zero(t, metricValue(snapshot, "vendor.active_power"))
	require.Len(t, snapshot.Metrics, 2)
	require.False(t, snapshot.Stale)

	// Also verify the raw TimescaleDB write via QueryTelemetry, confirming
	// the chain landed real rows and not just the Redis cache.
	records, err := e.Telemetry.Queries.QueryTelemetry.Handle(ctx, telemetryquery.QueryTelemetry{
		TenantID:  tenantID,
		CUCode:    cuCode,
		MetricID:  "electrical.active_power.v1",
		StartTime: ts.Add(-time.Minute),
		EndTime:   ts.Add(time.Minute),
	})
	require.NoError(t, err, "QueryTelemetry")
	require.NotEmpty(t, records, "expected at least one persisted telemetry_records row")
}

func metricValue(snapshot *telemetryquery.SnapshotView, metricID string) float64 {
	for _, metric := range snapshot.Metrics {
		if metric.MetricID == metricID {
			return metric.Value
		}
	}
	return 0
}
