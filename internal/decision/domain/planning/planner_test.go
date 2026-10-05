package planning

import (
	"context"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestPlannerAcceptsObjectiveAndContext(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	obj, err := objective.NewPowerObjective(objective.NewPowerObjectiveParams{
		ID:             "obj-1",
		TenantID:       "tenant-1",
		Scope:          policy.TargetScope{Type: port.ScopeAsset, ID: "asset-1"},
		MetricID:       contracts.MetricElectricalActivePowerSetpoint,
		TargetPowerKW:  -100,
		Window:         objective.TimeWindow{Start: start, End: start.Add(time.Hour)},
		Source:         objective.SourcePolicy,
		SourceID:       "policy-1",
		IdempotencyKey: "policy-1:charge:1",
		PolicyID:       "policy-1",
		PolicyVersion:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	dc, err := dctx.NewDecisionContext("tenant-1", obj.Scope, port.ResolvedScope{
		ScopeType:        port.ScopeAsset,
		ScopeID:          "asset-1",
		ResourceRevision: "rev-1",
		PrecheckOK:       true,
	}, dctx.ScopeState{Quality: port.QualityGood}, start)
	if err != nil {
		t.Fatal(err)
	}
	got, err := echoPlanner{}.Plan(context.Background(), obj, dc)
	if err != nil {
		t.Fatal(err)
	}
	if got.ObjectiveID != obj.ID || got.ResourceRevision != dc.ResourceRevision() {
		t.Fatalf("plan = %+v", got)
	}
}

type echoPlanner struct{}

func (echoPlanner) ID() string      { return "immediate" }
func (echoPlanner) Version() string { return "v1" }

func (echoPlanner) Plan(_ context.Context, obj objective.Objective, dc dctx.DecisionContext) (*plan.Plan, error) {
	power := obj.(*objective.PowerObjective)
	return plan.NewPlan(plan.NewPlanParams{
		ID:               "plan-1",
		ObjectiveID:      power.ID,
		TenantID:         power.TenantID,
		PolicyID:         power.PolicyID,
		PolicyVersion:    power.PolicyVersion,
		ResourceRevision: dc.ResourceRevision(),
		PlannerID:        "immediate",
		PlannerVersion:   "v1",
		Status:           plan.StatusRejected,
		Feasibility:      plan.FeasibilityInfeasible,
		UnmetPowerKW:     abs(power.TargetPowerKW),
		Window:           power.Window,
		GeneratedAt:      dc.CollectedAt,
	})
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

var _ Planner = echoPlanner{}
