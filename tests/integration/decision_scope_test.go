package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	decisioncommand "github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	decisionport "github.com/mushroomyuan/vpp-backend/decision/domain/port"
	gatewaycommand "github.com/mushroomyuan/vpp-backend/gateway/application/command"
	gatewaymodel "github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	resourcecommand "github.com/mushroomyuan/vpp-backend/resource/application/command"
	resourcemodel "github.com/mushroomyuan/vpp-backend/resource/domain/model"
	simdevice "github.com/mushroomyuan/vpp-backend/simulator/device"
	simdomain "github.com/mushroomyuan/vpp-backend/simulator/domain"
	telemetryquery "github.com/mushroomyuan/vpp-backend/telemetry/application/query"
	telemetrymodel "github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// TestDecision_CUAssetSiteCanonicalLoop is the cutover acceptance for Decision.
//
// One site holds two assets. The first asset mixes two storage CUs with a PV CU.
// The second asset holds one more storage CU. Simulator devices emit canonical
// MetricIDs. Gateway forwards those names unchanged. Decision then persists one
// Objective and Plan per CU, Asset, and Site policy, and Dispatch receives one
// task per plan. The asset and site tasks each contain every storage CU in that
// scope and none of the PV CU.
func TestDecision_CUAssetSiteCanonicalLoop(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()
	const tenantID = "tenant-decision-scope"
	const externalSystem = "simulator"

	site, err := e.Resource.Commands.CreateSite.Handle(ctx, resourcecommand.CreateSite{
		TenantID: tenantID,
		Name:     "scope-site",
	})
	require.NoError(t, err)
	asset1 := mustAsset(t, ctx, e, tenantID, site.SiteID, "asset-mixed")
	asset2 := mustAsset(t, ctx, e, tenantID, site.SiteID, "asset-storage")

	batA := mustStorageCU(t, ctx, e, tenantID, asset1, "bat-a")
	batB := mustStorageCU(t, ctx, e, tenantID, asset1, "bat-b")
	pv := mustPVCU(t, ctx, e, tenantID, asset1, "pv-1")
	batC := mustStorageCU(t, ctx, e, tenantID, asset2, "bat-c")
	storageIDs := map[string]struct{}{batA: {}, batB: {}, batC: {}}

	observedAt := time.Now().UTC().Truncate(time.Millisecond)
	for _, cu := range []seededCU{
		{id: batA, externalID: "sim-bat-a", kind: "battery"},
		{id: batB, externalID: "sim-bat-b", kind: "battery"},
		{id: batC, externalID: "sim-bat-c", kind: "battery"},
		{id: pv, externalID: "sim-pv-1", kind: "pv"},
	} {
		require.NoError(t, publishSimulatorSnapshot(ctx, e, tenantID, externalSystem, cu, observedAt))
	}

	views, err := e.Telemetry.Queries.GetSnapshots.Handle(ctx, telemetryquery.GetSnapshots{
		TenantID: tenantID,
		CUCodes:  []string{batA, batB, batC, pv},
		MetricIDs: []string{
			string(contracts.MetricEnergyStorageStateOfCharge),
			string(contracts.MetricElectricalActivePower),
		},
		StaleAge: 2 * time.Minute,
	})
	require.NoError(t, err)
	require.Len(t, views, 4)
	for _, view := range views {
		require.False(t, view.Stale, "cu %s", view.CUCode)
		require.NotEmpty(t, view.Metrics)
		for _, metric := range view.Metrics {
			require.Equal(t, telemetrymodel.QualityGood, metric.Quality, metric.MetricID)
			require.WithinDuration(t, observedAt, metric.ObservedAt, time.Second)
			if metric.MetricID == string(contracts.MetricEnergyStorageStateOfCharge) {
				require.Less(t, metric.Value, 80.0, "storage SOC must sit below the policy minimum")
			}
		}
	}

	cuPolicy := mustEnableSOC(t, ctx, e, tenantID, "cu-bat-c", decisionport.ScopeCU, batC, 40)
	assetPolicy := mustEnableSOC(t, ctx, e, tenantID, "asset-mixed", decisionport.ScopeAsset, asset1, 150)
	sitePolicy := mustEnableSOC(t, ctx, e, tenantID, "site-all", decisionport.ScopeSite, site.SiteID, 200)

	result, err := e.Cycle.Handle(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, result.Plans, 3)
	byPolicy := map[string]*plan.Plan{}
	for _, saved := range result.Plans {
		require.NotEmpty(t, saved.ID)
		require.NotEmpty(t, saved.ObjectiveID)
		require.Equal(t, plan.StatusReady, saved.Status)
		require.Equal(t, plan.FeasibilityFeasible, saved.Feasibility)
		byPolicy[saved.PolicyID] = saved
	}
	assertPlanCommands(t, byPolicy[cuPolicy.ID], map[string]struct{}{batC: {}})
	assertPlanCommands(t, byPolicy[assetPolicy.ID], map[string]struct{}{batA: {}, batB: {}})
	assertPlanCommands(t, byPolicy[sitePolicy.ID], storageIDs)
	for _, saved := range result.Plans {
		for _, step := range saved.Steps {
			for _, cmd := range step.Commands {
				_, storage := storageIDs[cmd.CUCode]
				require.True(t, storage, "command targeted a non-storage CU %s", cmd.CUCode)
			}
		}
	}

	require.NoError(t, e.Execution.Tick(ctx, time.Now().UTC()))
	for _, saved := range result.Plans {
		require.Len(t, saved.Steps, 1)
		step := saved.Steps[0]
		task, err := e.TaskRepo.FindByIdempotencyKey(ctx, tenantID, plan.StepIdempotencyKey(step.ID))
		require.NoError(t, err)
		require.Len(t, task.Actions, 1)
		require.Len(t, task.Actions[0].Commands, len(step.Commands))
		got := map[string]struct{}{}
		for _, cmd := range task.Actions[0].Commands {
			require.Equal(t, string(contracts.MetricElectricalActivePowerSetpoint), cmd.PointKey)
			require.NotNil(t, cmd.Value.FloatValue)
			require.Less(t, *cmd.Value.FloatValue, 0.0)
			got[cmd.CUCode] = struct{}{}
		}
		require.Equal(t, commandCUs(step), got)
	}
}

type seededCU struct {
	id         string
	externalID string
	kind       string
}

func mustAsset(t *testing.T, ctx context.Context, e *env, tenantID, siteID, name string) string {
	t.Helper()
	created, err := e.Resource.Commands.CreateAsset.Handle(ctx, resourcecommand.CreateAsset{
		TenantID: tenantID,
		SiteID:   siteID,
		Name:     name,
	})
	require.NoError(t, err)
	return created.AssetID
}

func mustStorageCU(t *testing.T, ctx context.Context, e *env, tenantID, assetID, name string) string {
	t.Helper()
	cuID := mustCU(t, ctx, e, tenantID, assetID, name, "battery")
	_, err := e.Resource.Commands.CreateCUCapability.Handle(ctx, resourcecommand.CreateCUCapability{
		TenantID:      tenantID,
		CUID:          cuID,
		CapabilityID:  string(contracts.CapabilityEnergyStorage),
		SchemaVersion: 1,
		Enabled:       true,
		Spec: map[string]any{
			"usable_energy_kwh":      100.0,
			"max_charge_power_kw":    80.0,
			"max_discharge_power_kw": 80.0,
		},
	})
	require.NoError(t, err)
	mustPoint(t, ctx, e, tenantID, assetID, cuID, contracts.MetricEnergyStorageStateOfCharge, resourcemodel.AccessModeRead)
	mustPoint(t, ctx, e, tenantID, assetID, cuID, contracts.MetricElectricalActivePower, resourcemodel.AccessModeRead)
	mustPoint(t, ctx, e, tenantID, assetID, cuID, contracts.MetricElectricalActivePowerSetpoint, resourcemodel.AccessModeWrite)
	return cuID
}

func mustPVCU(t *testing.T, ctx context.Context, e *env, tenantID, assetID, name string) string {
	t.Helper()
	cuID := mustCU(t, ctx, e, tenantID, assetID, name, "pv")
	_, err := e.Resource.Commands.CreateCUCapability.Handle(ctx, resourcecommand.CreateCUCapability{
		TenantID:      tenantID,
		CUID:          cuID,
		CapabilityID:  string(contracts.CapabilityPowerGeneration),
		SchemaVersion: 1,
		Enabled:       true,
		Spec: map[string]any{
			"rated_power_kw":         50.0,
			"min_power_kw":           0.0,
			"max_ramp_kw_per_minute": 10.0,
		},
	})
	require.NoError(t, err)
	mustPoint(t, ctx, e, tenantID, assetID, cuID, contracts.MetricElectricalActivePower, resourcemodel.AccessModeRead)
	return cuID
}

func mustCU(t *testing.T, ctx context.Context, e *env, tenantID, assetID, name, kind string) string {
	t.Helper()
	created, err := e.Resource.Commands.CreateCU.Handle(ctx, resourcecommand.CreateCU{
		TenantID: tenantID,
		ParentID: &assetID,
		Name:     name,
		Type:     kind,
	})
	require.NoError(t, err)
	return created.CUID
}

func mustPoint(
	t *testing.T,
	ctx context.Context,
	e *env,
	tenantID, assetID, cuID string,
	metric contracts.MetricID,
	access resourcemodel.AccessMode,
) {
	t.Helper()
	_, err := e.Resource.Commands.CreatePoint.Handle(ctx, resourcecommand.CreatePoint{
		TenantID:        tenantID,
		AssetID:         assetID,
		CUID:            cuID,
		MetricID:        string(metric),
		ExternalAddress: string(metric),
		AccessMode:      access,
		Scale:           1,
		Enabled:         true,
	})
	require.NoError(t, err)
}

func publishSimulatorSnapshot(ctx context.Context, e *env, tenantID, externalSystem string, cu seededCU, at time.Time) error {
	if _, err := e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID:       tenantID,
		ExternalSystem: externalSystem,
		ExternalID:     cu.externalID,
		CUCode:         cu.id,
	}); err != nil {
		return err
	}
	device := simdevice.New(simdomain.DeviceSpec{
		TenantID:   tenantID,
		CUCode:     cu.id,
		ExternalID: cu.externalID,
		Name:       cu.externalID,
		Type:       cu.kind,
		Points:     simulatorPoints(cu.kind),
	})
	snapshot := device.Snapshot()
	metrics := make([]gatewaymodel.ExternalMetric, 0, len(snapshot))
	for _, point := range snapshot {
		metrics = append(metrics, gatewaymodel.ExternalMetric{Name: point.PointKey, Value: point.Value})
	}
	_, err := e.Gateway.Commands.ReceiveTelemetry.Handle(ctx, gatewaycommand.ReceiveTelemetry{
		Telemetry: &gatewaymodel.ExternalTelemetry{
			TenantID:       tenantID,
			ExternalSystem: externalSystem,
			ExternalID:     cu.externalID,
			Timestamp:      at,
			Metrics:        metrics,
		},
	})
	return err
}

func simulatorPoints(kind string) []simdomain.PointDef {
	power := simdomain.PointDef{PointKey: string(contracts.MetricElectricalActivePower)}
	if kind != "battery" {
		return []simdomain.PointDef{power}
	}
	return []simdomain.PointDef{
		{PointKey: string(contracts.MetricEnergyStorageStateOfCharge)},
		power,
		{PointKey: string(contracts.MetricElectricalActivePowerSetpoint), ControlFlag: true},
	}
}

func mustEnableSOC(
	t *testing.T,
	ctx context.Context,
	e *env,
	tenantID, name string,
	scopeType decisionport.ScopeType,
	scopeID string,
	chargeKW float64,
) *policy.Policy {
	t.Helper()
	created, err := e.Policies.Commands.CreatePolicy.Handle(ctx, decisioncommand.CreatePolicy{
		TenantID: tenantID,
		Name:     name,
		Kind:     policy.KindSOCThreshold,
		Scope:    policy.TargetScope{Type: scopeType, ID: scopeID},
		Cooldown: time.Minute,
		SOC: &policy.SOCThresholdSpec{
			MinSOC:           80,
			MaxSOC:           95,
			ChargePowerKW:    chargeKW,
			DischargePowerKW: chargeKW,
		},
	})
	require.NoError(t, err)
	enabled, err := e.Policies.Commands.EnablePolicy.Handle(ctx, decisioncommand.EnablePolicy{
		TenantID: tenantID,
		ID:       created.ID,
		Version:  created.Version,
	})
	require.NoError(t, err)
	require.True(t, enabled.Enabled)
	return enabled
}

func assertPlanCommands(t *testing.T, saved *plan.Plan, want map[string]struct{}) {
	t.Helper()
	require.NotNil(t, saved)
	require.Len(t, saved.Steps, 1)
	got := commandCUs(saved.Steps[0])
	require.Equal(t, want, got)
	for _, cmd := range saved.Steps[0].Commands {
		require.Equal(t, contracts.MetricElectricalActivePowerSetpoint, cmd.MetricID)
		require.NotNil(t, cmd.Value.FloatValue)
		require.Less(t, *cmd.Value.FloatValue, 0.0)
	}
}

func commandCUs(step plan.PlanStep) map[string]struct{} {
	got := make(map[string]struct{}, len(step.Commands))
	for _, cmd := range step.Commands {
		got[cmd.CUCode] = struct{}{}
	}
	return got
}
