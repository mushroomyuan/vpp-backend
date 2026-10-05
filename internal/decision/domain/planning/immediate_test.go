package planning

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestImmediatePlanner_SingleStepAtCollectionTime(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	obj := mustObjective(t, -40, at)
	dc := mustEmptyContext(t, at)
	value := -40.0
	planner := NewImmediatePlanner(stubAllocator{result: allocation.Result{
		Feasibility: plan.FeasibilityFeasible,
		Commands: []plan.PlannedCommand{{
			CUCode:          "cu-1",
			MetricID:        contracts.MetricElectricalActivePowerSetpoint,
			Value:           model.FloatCommandValue(value),
			BindingRevision: 2,
		}},
	}}, seq())
	got, err := planner.Plan(context.Background(), obj, dc)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != plan.StatusReady || got.PlannerID != ImmediatePlannerID || len(got.Steps) != 1 {
		t.Fatalf("plan = %+v", got)
	}
	step := got.Steps[0]
	if !step.ExecuteAt.Equal(at) || step.Ordinal != 1 || len(step.Commands) != 1 || step.Commands[0].ID == "" {
		t.Fatalf("step = %+v", step)
	}
	if step.Commands[0].MetricID != contracts.MetricElectricalActivePowerSetpoint || *step.Commands[0].Value.FloatValue != -40 {
		t.Fatalf("command = %+v", step.Commands[0])
	}
}

func TestImmediatePlanner_RejectedPlanHasNoSteps(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	planner := NewImmediatePlanner(stubAllocator{result: allocation.Result{
		Feasibility:  plan.FeasibilityInfeasible,
		UnmetPowerKW: 40,
	}}, seq())
	got, err := planner.Plan(context.Background(), mustObjective(t, -40, at), mustEmptyContext(t, at))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != plan.StatusRejected || len(got.Steps) != 0 || got.UnmetPowerKW != 40 {
		t.Fatalf("plan = %+v", got)
	}
}

type stubAllocator struct {
	result allocation.Result
}

func (s stubAllocator) Allocate(context.Context, *objective.PowerObjective, dctx.DecisionContext) (allocation.Result, error) {
	return s.result, nil
}

func mustObjective(t *testing.T, target float64, at time.Time) *objective.PowerObjective {
	t.Helper()
	obj, err := objective.NewPowerObjective(objective.NewPowerObjectiveParams{
		ID: "obj-1", TenantID: "tenant-1",
		Scope:          policy.TargetScope{Type: port.ScopeCU, ID: "cu-1"},
		MetricID:       contracts.MetricElectricalActivePowerSetpoint,
		TargetPowerKW:  target,
		Window:         objective.TimeWindow{Start: at, End: at.Add(time.Minute)},
		Source:         objective.SourcePolicy,
		SourceID:       "policy-1",
		IdempotencyKey: "policy-1:charge:1",
		PolicyID:       "policy-1",
		PolicyVersion:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return obj
}

func mustEmptyContext(t *testing.T, at time.Time) dctx.DecisionContext {
	t.Helper()
	dc, err := dctx.NewDecisionContext("tenant-1", policy.TargetScope{Type: port.ScopeCU, ID: "cu-1"}, port.ResolvedScope{
		ScopeType: port.ScopeCU, ScopeID: "cu-1", ResourceRevision: "rev-1", PrecheckOK: true,
	}, dctx.ScopeState{Quality: port.QualityGood}, at)
	if err != nil {
		t.Fatal(err)
	}
	return dc
}

func seq() func() string {
	n := 0
	return func() string {
		n++
		return "id"
	}
}
