package allocation

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestHeuristicAllocator_SplitsByPowerCapAndSOCMargin(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dc := mustDC(t, []cuSpec{
		{id: "cu-a", soc: 20, energy: 100, charge: 100, discharge: 100},
		{id: "cu-b", soc: 80, energy: 100, charge: 100, discharge: 100},
	}, at)
	got := mustAllocate(t, -100, dc)
	if got.Feasibility != plan.FeasibilityFeasible || got.UnmetPowerKW != 0 {
		t.Fatalf("result = %+v", got)
	}
	values := commandValues(t, got.Commands)
	// Headroom 80 kWh and 20 kWh, neither cap binds.
	if !near(values["cu-a"], -80) || !near(values["cu-b"], -20) {
		t.Fatalf("shares = %+v", values)
	}
	for _, cmd := range got.Commands {
		if cmd.MetricID != contracts.MetricElectricalActivePowerSetpoint {
			t.Fatalf("metric = %s", cmd.MetricID)
		}
	}
}

func TestHeuristicAllocator_RedistributesWhenACapBinds(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dc := mustDC(t, []cuSpec{
		{id: "cu-a", soc: 20, energy: 100, charge: 30, discharge: 30},
		{id: "cu-b", soc: 80, energy: 100, charge: 100, discharge: 100},
	}, at)
	got := mustAllocate(t, -100, dc)
	values := commandValues(t, got.Commands)
	if !near(values["cu-a"], -30) || !near(values["cu-b"], -70) || got.Feasibility != plan.FeasibilityFeasible {
		t.Fatalf("shares = %+v feasibility %s", values, got.Feasibility)
	}
}

func TestHeuristicAllocator_PartialWhenFeasiblePowerIsShort(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dc := mustDC(t, []cuSpec{
		{id: "cu-a", soc: 50, energy: 100, charge: 30, discharge: 30},
		{id: "cu-b", soc: 50, energy: 100, charge: 20, discharge: 20},
	}, at)
	got := mustAllocate(t, -100, dc)
	if got.Feasibility != plan.FeasibilityPartiallyFeasible || !near(got.UnmetPowerKW, 50) {
		t.Fatalf("result feasibility=%s unmet=%v", got.Feasibility, got.UnmetPowerKW)
	}
	values := commandValues(t, got.Commands)
	if !near(values["cu-a"], -30) || !near(values["cu-b"], -20) {
		t.Fatalf("shares = %+v", values)
	}
}

func TestHeuristicAllocator_ZeroFeasiblePowerRejects(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	min := 0.0
	max := 50.0
	dc := mustDC(t, []cuSpec{
		{id: "cu-a", soc: 10, energy: 100, charge: 100, discharge: 100, safety: &port.SafetyConstraint{
			MinValue: &min, MaxValue: &max, Version: 2,
		}},
	}, at)
	got := mustAllocate(t, -100, dc)
	if got.Feasibility != plan.FeasibilityInfeasible || len(got.Commands) != 0 || !near(got.UnmetPowerKW, 100) {
		t.Fatalf("result = %+v commands %d", got.Feasibility, len(got.Commands))
	}
}

func TestHeuristicAllocator_ClampsDischargeToSafety(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	max := 40.0
	dc := mustDC(t, []cuSpec{
		{id: "cu-a", soc: 90, energy: 100, charge: 100, discharge: 100, safety: &port.SafetyConstraint{
			MaxValue: &max, Version: 4,
		}},
	}, at)
	got := mustAllocate(t, 80, dc)
	values := commandValues(t, got.Commands)
	if !near(values["cu-a"], 40) || got.Feasibility != plan.FeasibilityPartiallyFeasible || !near(got.UnmetPowerKW, 40) {
		t.Fatalf("shares = %+v unmet %v feasibility %s", values, got.UnmetPowerKW, got.Feasibility)
	}
	if got.Commands[0].BindingRevision != 3 || got.Commands[0].Safety == nil || *got.Commands[0].Safety.MaxValue != 40 {
		t.Fatalf("command = %+v", got.Commands[0])
	}
}

func mustAllocate(t *testing.T, target float64, dc dctx.DecisionContext) Result {
	t.Helper()
	obj, err := objective.NewPowerObjective(objective.NewPowerObjectiveParams{
		ID:             "obj-1",
		TenantID:       "tenant-1",
		Scope:          dc.Scope,
		MetricID:       contracts.MetricElectricalActivePowerSetpoint,
		TargetPowerKW:  target,
		Window:         objective.TimeWindow{Start: dc.CollectedAt, End: dc.CollectedAt.Add(time.Minute)},
		Source:         objective.SourcePolicy,
		SourceID:       "policy-1",
		IdempotencyKey: "policy-1:test",
		PolicyID:       "policy-1",
		PolicyVersion:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewHeuristicAllocator().Allocate(context.Background(), obj, dc)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

type cuSpec struct {
	id                string
	soc               float64
	energy            float64
	charge, discharge float64
	safety            *port.SafetyConstraint
}

func mustDC(t *testing.T, cus []cuSpec, at time.Time) dctx.DecisionContext {
	t.Helper()
	scope := policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"}
	if len(cus) == 1 {
		scope = policy.TargetScope{Type: port.ScopeCU, ID: cus[0].id}
	}
	members := make([]port.ResolvedCU, len(cus))
	units := make([]dctx.CUState, len(cus))
	for i, cu := range cus {
		members[i] = port.ResolvedCU{
			CUID: cu.id, AssetID: "asset-1", Lifecycle: port.LifecycleActive,
			Capabilities: []port.ResolvedCapability{{
				CapabilityID: contracts.CapabilityEnergyStorage, SchemaVersion: 1, Enabled: true,
				Spec: map[string]any{
					"usable_energy_kwh":      cu.energy,
					"max_charge_power_kw":    cu.charge,
					"max_discharge_power_kw": cu.discharge,
				},
			}},
			Bindings: []port.ResolvedBinding{{
				MetricID: contracts.MetricElectricalActivePowerSetpoint, AccessMode: port.BindingAccessWrite,
				Enabled: true, Revision: 3, Safety: cu.safety,
			}},
		}
		units[i] = dctx.CUState{CUCode: cu.id, Metrics: []port.MetricSample{{
			MetricID: contracts.MetricEnergyStorageStateOfCharge, Value: cu.soc, ObservedAt: at, Quality: port.QualityGood,
		}}}
	}
	dc, err := dctx.NewDecisionContext("tenant-1", scope, port.ResolvedScope{
		ScopeType: scope.Type, ScopeID: scope.ID, ResourceRevision: "rev-1", PrecheckOK: true, Members: members,
	}, dctx.ScopeState{Quality: port.QualityGood, Units: units}, at)
	if err != nil {
		t.Fatal(err)
	}
	return dc
}

func commandValues(t *testing.T, commands []plan.PlannedCommand) map[string]float64 {
	t.Helper()
	out := map[string]float64{}
	for _, cmd := range commands {
		if cmd.Value.FloatValue == nil {
			t.Fatalf("command %+v has no float", cmd)
		}
		out[cmd.CUCode] = *cmd.Value.FloatValue
	}
	return out
}

func near(got, want float64) bool {
	return math.Abs(got-want) < 1e-6
}
