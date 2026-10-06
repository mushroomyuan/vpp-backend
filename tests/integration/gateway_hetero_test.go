package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	decisioncommand "github.com/mushroomyuan/vpp-backend/decision/application/command"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	decisionport "github.com/mushroomyuan/vpp-backend/decision/domain/port"
	dispatchcommand "github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	dispatchquery "github.com/mushroomyuan/vpp-backend/dispatch/application/query"
	dispatchmodel "github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	gatewaycommand "github.com/mushroomyuan/vpp-backend/gateway/application/command"
	gatewaydomain "github.com/mushroomyuan/vpp-backend/gateway/domain"
	gatewaymodel "github.com/mushroomyuan/vpp-backend/gateway/domain/model"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	resourcecommand "github.com/mushroomyuan/vpp-backend/resource/application/command"
	resourcemodel "github.com/mushroomyuan/vpp-backend/resource/domain/model"
	telemetryquery "github.com/mushroomyuan/vpp-backend/telemetry/application/query"
)

// TestGateway_TwoVendorsCanonicalBinding is the acceptance for Gateway translation.
//
// Vendor A reports power in watts and SOC in permille, and its setpoint register
// uses the same watt scale. Vendor B reports power in kW with the opposite sign
// and SOC in percent. Telemetry stores only canonical MetricIDs. A canonical
// charge setpoint is inverse-converted back to each vendor's address and raw
// units. Renaming an external address leaves the policy on the canonical metric,
// and a command outside the safety limit is not sent.
func TestGateway_TwoVendorsCanonicalBinding(t *testing.T) {
	e := sharedEnv
	ctx := context.Background()
	const tenantID = "tenant-gateway-hetero"

	site, err := e.Resource.Commands.CreateSite.Handle(ctx, resourcecommand.CreateSite{
		TenantID: tenantID,
		Name:     "hetero-site",
	})
	require.NoError(t, err)
	assetID := mustAsset(t, ctx, e, tenantID, site.SiteID, "hetero-asset")

	vendorA := seedVendorCU(t, ctx, e, tenantID, assetID, "vendor-a", "ems-a", "device-a", []vendorPointSpec{
		{metric: contracts.MetricElectricalActivePower, address: "HOLDING_40001", access: resourcemodel.AccessModeRead, scale: 0.001},
		{metric: contracts.MetricEnergyStorageStateOfCharge, address: "SOC_PERMILLE", access: resourcemodel.AccessModeRead, scale: 0.1},
		{metric: contracts.MetricElectricalActivePowerSetpoint, address: "CMD_REG_40010", access: resourcemodel.AccessModeWrite, scale: 0.001, safety: powerSafety()},
	})
	vendorB := seedVendorCU(t, ctx, e, tenantID, assetID, "vendor-b", "ems-b", "device-b", []vendorPointSpec{
		{metric: contracts.MetricElectricalActivePower, address: "inv.p.active", access: resourcemodel.AccessModeRead, scale: -1},
		{metric: contracts.MetricEnergyStorageStateOfCharge, address: "bms.soc", access: resourcemodel.AccessModeRead, scale: 1},
		{metric: contracts.MetricElectricalActivePowerSetpoint, address: "inv.p.set", access: resourcemodel.AccessModeWrite, scale: -1, safety: powerSafety()},
	})

	observedAt := time.Now().UTC().Truncate(time.Millisecond)
	ingestVendor(t, ctx, e, tenantID, vendorA, observedAt, map[string]float64{
		"HOLDING_40001": 25000,
		"SOC_PERMILLE":  200,
		"A_SPARE":       1,
	}, 2, 1)
	ingestVendor(t, ctx, e, tenantID, vendorB, observedAt, map[string]float64{
		"inv.p.active": -25,
		"bms.soc":      20,
		"B_SPARE":      1,
	}, 2, 1)

	snapA := waitSnapshot(t, ctx, e, tenantID, vendorA.cuID)
	snapB := waitSnapshot(t, ctx, e, tenantID, vendorB.cuID)
	require.InDelta(t, 25, metricValue(snapA, string(contracts.MetricElectricalActivePower)), 0.001)
	require.InDelta(t, 20, metricValue(snapA, string(contracts.MetricEnergyStorageStateOfCharge)), 0.001)
	require.InDelta(t, 25, metricValue(snapB, string(contracts.MetricElectricalActivePower)), 0.001)
	require.InDelta(t, 20, metricValue(snapB, string(contracts.MetricEnergyStorageStateOfCharge)), 0.001)
	for _, name := range []string{"HOLDING_40001", "SOC_PERMILLE", "CMD_REG_40010", "A_SPARE", "inv.p.active", "bms.soc", "inv.p.set", "B_SPARE"} {
		require.Zero(t, metricValue(snapA, name), name)
		require.Zero(t, metricValue(snapB, name), name)
	}
	require.Len(t, snapA.Metrics, 2)
	require.Len(t, snapB.Metrics, 2)

	sentA := executeCanonical(t, ctx, e, tenantID, vendorA.cuID, -10, 0)
	require.Equal(t, "ems-a", sentA.ExternalSystem)
	require.Equal(t, "device-a", sentA.ExternalID)
	require.Equal(t, "CMD_REG_40010", sentA.Address)
	require.InDelta(t, -10000, sentA.Value, 0.05)

	sentB := executeCanonical(t, ctx, e, tenantID, vendorB.cuID, -10, 0)
	require.Equal(t, "ems-b", sentB.ExternalSystem)
	require.Equal(t, "device-b", sentB.ExternalID)
	require.Equal(t, "inv.p.set", sentB.Address)
	require.InDelta(t, 10, sentB.Value, 0.001)

	taskID := submitSetpoint(t, ctx, e, tenantID, vendorB.cuID, -10)
	var task *dispatchmodel.DispatchTask
	requireEventuallyf(t, func() bool {
		res, qErr := e.Dispatch.Queries.GetTask.Handle(ctx, dispatchquery.GetTask{TenantID: tenantID, TaskID: taskID})
		if qErr != nil || res.Task == nil || !res.Task.IsFinished() {
			return false
		}
		task = res.Task
		return true
	}, "dispatch task %s did not finish", taskID)
	require.Equal(t, dispatchmodel.TaskStatusCompleted, task.Status)
	dispatched := task.Actions[0].Commands[0]
	require.Equal(t, string(contracts.MetricElectricalActivePowerSetpoint), dispatched.PointKey)
	require.NotNil(t, dispatched.Value.FloatValue)
	require.InDelta(t, -10, *dispatched.Value.FloatValue, 0.001)
	forwarded, ok := e.SentCommands.byID(dispatched.ID)
	require.True(t, ok)
	require.Equal(t, "inv.p.set", forwarded.Address)
	require.InDelta(t, 10, forwarded.Value, 0.001)

	overLimit := gatewaycommand.ExecuteCommand{
		CommandID: idgen.Must(), TenantID: tenantID, CUCode: vendorA.cuID,
		PointKey: string(contracts.MetricElectricalActivePowerSetpoint), Value: 200,
	}
	_, err = e.Gateway.Commands.ExecuteCommand.Handle(ctx, overLimit)
	require.ErrorIs(t, err, gatewaydomain.ErrCommandRejected)
	_, sentOver := e.SentCommands.byID(overLimit.CommandID)
	require.False(t, sentOver)

	_, err = e.Resource.Commands.UpdatePoint.Handle(ctx, resourcecommand.UpdatePoint{
		ID: vendorA.points[contracts.MetricElectricalActivePowerSetpoint], TenantID: tenantID,
		MetricID: string(contracts.MetricElectricalActivePowerSetpoint), ExternalAddress: "CMD_REG_40011",
		AccessMode: resourcemodel.AccessModeWrite, Scale: 0.001, Enabled: true,
		SafetyConstraint: powerSafety(), ExpectedRevision: 1,
	})
	require.NoError(t, err)

	var renamed recordedCommand
	requireEventuallyf(t, func() bool {
		cmd := gatewaycommand.ExecuteCommand{
			CommandID: idgen.Must(), TenantID: tenantID, CUCode: vendorA.cuID,
			PointKey: string(contracts.MetricElectricalActivePowerSetpoint), Value: -10,
			BindingRevision: 2,
		}
		if _, execErr := e.Gateway.Commands.ExecuteCommand.Handle(ctx, cmd); execErr != nil {
			return false
		}
		got, found := e.SentCommands.byID(cmd.CommandID)
		if !found || got.Address != "CMD_REG_40011" {
			return false
		}
		renamed = got
		return true
	}, "renamed setpoint was not delivered")
	require.InDelta(t, -10000, renamed.Value, 0.05)

	staleRev := gatewaycommand.ExecuteCommand{
		CommandID: idgen.Must(), TenantID: tenantID, CUCode: vendorA.cuID,
		PointKey: string(contracts.MetricElectricalActivePowerSetpoint), Value: -10,
		BindingRevision: 1,
	}
	_, err = e.Gateway.Commands.ExecuteCommand.Handle(ctx, staleRev)
	require.ErrorIs(t, err, gatewaydomain.ErrCommandRejected)
	_, sentStale := e.SentCommands.byID(staleRev.CommandID)
	require.False(t, sentStale)

	enabled := mustEnableSOC(t, ctx, e, tenantID, "hetero-cu-a", decisionport.ScopeCU, vendorA.cuID, 40)
	// A one-millisecond cooldown lets the same policy fire again after the address change.
	updated, err := e.Policies.Commands.UpdatePolicy.Handle(ctx, decisioncommand.UpdatePolicy{
		TenantID: tenantID, ID: enabled.ID, Name: enabled.Name, Version: enabled.Version,
		Scope: enabled.Scope, Cooldown: time.Millisecond, SOC: enabled.SOC,
	})
	require.NoError(t, err)
	require.True(t, updated.Enabled)

	first := runHeteroPlan(t, ctx, e, updated.ID)
	assertCanonicalCharge(t, first, vendorA.cuID)

	_, err = e.Resource.Commands.UpdatePoint.Handle(ctx, resourcecommand.UpdatePoint{
		ID: vendorA.points[contracts.MetricEnergyStorageStateOfCharge], TenantID: tenantID,
		MetricID: string(contracts.MetricEnergyStorageStateOfCharge), ExternalAddress: "SOC_PM_V2",
		AccessMode: resourcemodel.AccessModeRead, Scale: 0.1, Enabled: true, ExpectedRevision: 1,
	})
	require.NoError(t, err)
	requireEventuallyf(t, func() bool {
		res, ingestErr := e.Gateway.Commands.ReceiveTelemetry.Handle(ctx, gatewaycommand.ReceiveTelemetry{
			Telemetry: vendorTelemetry(tenantID, vendorA, time.Now().UTC(), map[string]float64{"SOC_PM_V2": 200}),
		})
		return ingestErr == nil && res.Accepted == 1 && res.Isolated == 0
	}, "renamed SOC address was not accepted")
	old := ingestVendorResult(t, ctx, e, tenantID, vendorA, time.Now().UTC(), map[string]float64{"SOC_PERMILLE": 200})
	require.Equal(t, 0, old.Accepted)
	require.Equal(t, 1, old.Isolated)
	afterRename := waitSnapshot(t, ctx, e, tenantID, vendorA.cuID)
	require.InDelta(t, 20, metricValue(afterRename, string(contracts.MetricEnergyStorageStateOfCharge)), 0.001)
	require.Zero(t, metricValue(afterRename, "SOC_PM_V2"))
	require.Zero(t, metricValue(afterRename, "SOC_PERMILLE"))

	time.Sleep(200 * time.Millisecond)
	second := runHeteroPlan(t, ctx, e, updated.ID)
	assertCanonicalCharge(t, second, vendorA.cuID)
	require.NotEqual(t, first.ID, second.ID)
}

func powerSafety() *resourcemodel.PointSafetyConstraint {
	minPower := -80.0
	maxPower := 80.0
	return &resourcemodel.PointSafetyConstraint{MinValue: &minPower, MaxValue: &maxPower, Version: 1}
}

type vendorPointSpec struct {
	metric  contracts.MetricID
	address string
	access  resourcemodel.AccessMode
	scale   float64
	offset  float64
	safety  *resourcemodel.PointSafetyConstraint
}

type seededVendor struct {
	cuID   string
	system string
	device string
	points map[contracts.MetricID]string
}

func seedVendorCU(
	t *testing.T,
	ctx context.Context,
	e *env,
	tenantID, assetID, name, system, device string,
	points []vendorPointSpec,
) seededVendor {
	t.Helper()
	cuID := mustCU(t, ctx, e, tenantID, assetID, name, "battery")
	_, err := e.Resource.Commands.CreateCUCapability.Handle(ctx, resourcecommand.CreateCUCapability{
		TenantID: tenantID, CUID: cuID,
		CapabilityID:  string(contracts.CapabilityEnergyStorage),
		SchemaVersion: 1, Enabled: true,
		Spec: map[string]any{
			"usable_energy_kwh":      100.0,
			"max_charge_power_kw":    80.0,
			"max_discharge_power_kw": 80.0,
		},
	})
	require.NoError(t, err)
	ids := map[contracts.MetricID]string{}
	for _, point := range points {
		created, err := e.Resource.Commands.CreatePoint.Handle(ctx, resourcecommand.CreatePoint{
			TenantID: tenantID, AssetID: assetID, CUID: cuID,
			MetricID: string(point.metric), ExternalAddress: point.address,
			AccessMode: point.access, Scale: point.scale, Offset: point.offset,
			Enabled: true, SafetyConstraint: point.safety,
		})
		require.NoError(t, err)
		ids[point.metric] = created.PointID
	}
	_, err = e.Gateway.Commands.CreateMapping.Handle(ctx, gatewaycommand.CreateMapping{
		TenantID: tenantID, ExternalSystem: system, ExternalID: device, CUCode: cuID,
	})
	require.NoError(t, err)
	return seededVendor{cuID: cuID, system: system, device: device, points: ids}
}

func ingestVendor(
	t *testing.T,
	ctx context.Context,
	e *env,
	tenantID string,
	vendor seededVendor,
	at time.Time,
	raw map[string]float64,
	accepted, isolated int,
) {
	t.Helper()
	res := ingestVendorResult(t, ctx, e, tenantID, vendor, at, raw)
	require.Equal(t, accepted, res.Accepted)
	require.Equal(t, isolated, res.Isolated)
}

func ingestVendorResult(
	t *testing.T,
	ctx context.Context,
	e *env,
	tenantID string,
	vendor seededVendor,
	at time.Time,
	raw map[string]float64,
) *gatewaycommand.ReceiveTelemetryResult {
	t.Helper()
	res, err := e.Gateway.Commands.ReceiveTelemetry.Handle(ctx, gatewaycommand.ReceiveTelemetry{
		Telemetry: vendorTelemetry(tenantID, vendor, at, raw),
	})
	require.NoError(t, err)
	return res
}

func vendorTelemetry(tenantID string, vendor seededVendor, at time.Time, raw map[string]float64) *gatewaymodel.ExternalTelemetry {
	metrics := make([]gatewaymodel.ExternalMetric, 0, len(raw))
	for address, value := range raw {
		metrics = append(metrics, gatewaymodel.ExternalMetric{ExternalAddress: address, Value: value})
	}
	return &gatewaymodel.ExternalTelemetry{
		TenantID: tenantID, ExternalSystem: vendor.system, ExternalID: vendor.device,
		Timestamp: at, Metrics: metrics,
	}
}

func waitSnapshot(t *testing.T, ctx context.Context, e *env, tenantID, cuID string) *telemetryquery.SnapshotView {
	t.Helper()
	var snapshot *telemetryquery.SnapshotView
	requireEventuallyf(t, func() bool {
		res, err := e.Telemetry.Queries.GetSnapshot.Handle(ctx, telemetryquery.GetSnapshot{
			TenantID: tenantID, CUCode: cuID,
		})
		if err != nil || res == nil || len(res.Metrics) == 0 {
			return false
		}
		snapshot = res
		return true
	}, "snapshot for %s never appeared", cuID)
	return snapshot
}

func executeCanonical(t *testing.T, ctx context.Context, e *env, tenantID, cuID string, canonical float64, revision int64) recordedCommand {
	t.Helper()
	cmd := gatewaycommand.ExecuteCommand{
		CommandID: idgen.Must(), TenantID: tenantID, CUCode: cuID,
		PointKey: string(contracts.MetricElectricalActivePowerSetpoint), Value: canonical,
		BindingRevision: revision,
	}
	_, err := e.Gateway.Commands.ExecuteCommand.Handle(ctx, cmd)
	require.NoError(t, err)
	sent, ok := e.SentCommands.byID(cmd.CommandID)
	require.True(t, ok)
	return sent
}

func submitSetpoint(t *testing.T, ctx context.Context, e *env, tenantID, cuID string, canonical float64) string {
	t.Helper()
	result, err := e.Dispatch.Commands.SubmitTask.Handle(ctx, dispatchcommand.SubmitTask{
		TenantID: tenantID,
		Name:     "hetero-setpoint",
		Actions: []dispatchcommand.SubmitActionDTO{{
			Name: "set", Sequence: 1,
			Commands: []dispatchcommand.SubmitCommandDTO{{
				CUCode:   cuID,
				PointKey: string(contracts.MetricElectricalActivePowerSetpoint),
				Value:    dispatchmodel.FloatCommandValue(canonical),
			}},
		}},
	})
	require.NoError(t, err)
	return result.TaskID
}

func runHeteroPlan(t *testing.T, ctx context.Context, e *env, policyID string) *plan.Plan {
	t.Helper()
	result, err := e.Cycle.Handle(ctx, time.Now().UTC())
	require.NoError(t, err)
	for _, saved := range result.Plans {
		if saved.PolicyID == policyID {
			return saved
		}
	}
	t.Fatalf("policy %s did not produce a plan", policyID)
	return nil
}

func assertCanonicalCharge(t *testing.T, saved *plan.Plan, cuID string) {
	t.Helper()
	require.Equal(t, plan.StatusReady, saved.Status)
	require.Len(t, saved.Steps, 1)
	require.Len(t, saved.Steps[0].Commands, 1)
	cmd := saved.Steps[0].Commands[0]
	require.Equal(t, cuID, cmd.CUCode)
	require.Equal(t, contracts.MetricElectricalActivePowerSetpoint, cmd.MetricID)
	require.NotNil(t, cmd.Value.FloatValue)
	require.InDelta(t, -40, *cmd.Value.FloatValue, 0.001)
	for _, vendorName := range []string{"HOLDING_40001", "SOC_PERMILLE", "SOC_PM_V2", "CMD_REG_40010", "CMD_REG_40011", "inv.p.set", "bms.soc"} {
		require.NotEqual(t, vendorName, string(cmd.MetricID))
	}
}
