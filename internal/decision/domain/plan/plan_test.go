package plan

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
)

func TestNewPlan_Feasible(t *testing.T) {
	p, err := NewPlan(feasibleParams())
	if err != nil {
		t.Fatalf("NewPlan: %v", err)
	}
	if p.Status != StatusReady || len(p.Steps) != 1 || len(p.Steps[0].Commands) != 1 {
		t.Fatalf("plan = %+v", p)
	}
	min := 0.0
	input := feasibleParams()
	input.Steps[0].Commands[0].Safety = &port.SafetyConstraint{MinValue: &min, Version: 2}
	copied, err := NewPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	*copied.Steps[0].Commands[0].Safety.MinValue = 5
	if *input.Steps[0].Commands[0].Safety.MinValue != 0 {
		t.Fatal("constructor must copy the safety snapshot")
	}
}

func TestNewPlan_PartialAndRejected(t *testing.T) {
	partial := feasibleParams()
	partial.Feasibility = FeasibilityPartiallyFeasible
	partial.UnmetPowerKW = 20
	if _, err := NewPlan(partial); err != nil {
		t.Fatal(err)
	}

	rejected := feasibleParams()
	rejected.Status = StatusRejected
	rejected.Feasibility = FeasibilityInfeasible
	rejected.UnmetPowerKW = 100
	rejected.Steps = nil
	if _, err := NewPlan(rejected); err != nil {
		t.Fatal(err)
	}

	stale := feasibleParams()
	stale.Status = StatusStale
	stale.Steps[0].Status = StatusStale
	if _, err := NewPlan(stale); err != nil {
		t.Fatal(err)
	}

	submitted := feasibleParams()
	submitted.Status = StatusSubmitted
	submitted.Steps[0].Status = StatusSubmitted
	if _, err := NewPlan(submitted); err != nil {
		t.Fatal(err)
	}
}

func TestNewPlan_Rejects(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*NewPlanParams)
		want   string
	}{
		{name: "feasible with unmet", mutate: func(p *NewPlanParams) { p.UnmetPowerKW = 1 }, want: "unmet power"},
		{name: "partial without unmet", mutate: func(p *NewPlanParams) {
			p.Feasibility = FeasibilityPartiallyFeasible
		}, want: "unmet power"},
		{name: "rejected with a step", mutate: func(p *NewPlanParams) {
			p.Status = StatusRejected
			p.Feasibility = FeasibilityInfeasible
			p.UnmetPowerKW = 10
		}, want: "no steps"},
		{name: "ready without steps", mutate: func(p *NewPlanParams) { p.Steps = nil }, want: "at least one step"},
		{name: "ordinal gap", mutate: func(p *NewPlanParams) { p.Steps[0].Ordinal = 2 }, want: "ordinals"},
		{name: "non float value", mutate: func(p *NewPlanParams) {
			p.Steps[0].Commands[0].Value = model.BoolCommandValue(true)
		}, want: "finite float"},
		{name: "two value fields", mutate: func(p *NewPlanParams) {
			v := model.FloatCommandValue(-40)
			b := true
			v.BoolValue = &b
			p.Steps[0].Commands[0].Value = v
		}, want: "exactly one"},
		{name: "duplicate target", mutate: func(p *NewPlanParams) {
			extra := p.Steps[0].Commands[0]
			extra.ID = "cmd-2"
			p.Steps[0].Commands = append(p.Steps[0].Commands, extra)
		}, want: "duplicate command"},
		{name: "zero binding revision", mutate: func(p *NewPlanParams) {
			p.Steps[0].Commands[0].BindingRevision = 0
		}, want: "binding_revision"},
		{name: "soc metric", mutate: func(p *NewPlanParams) {
			p.Steps[0].Commands[0].MetricID = contracts.MetricEnergyStorageStateOfCharge
		}, want: "writable kW"},
		{name: "safety inverted", mutate: func(p *NewPlanParams) {
			min, max := 10.0, 1.0
			p.Steps[0].Commands[0].Safety = &port.SafetyConstraint{MinValue: &min, MaxValue: &max, Version: 1}
		}, want: "min_value"},
		{name: "policy version without id", mutate: func(p *NewPlanParams) {
			p.PolicyID = ""
			p.PolicyVersion = 2
		}, want: "policy_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := feasibleParams()
			tt.mutate(&params)
			_, err := NewPlan(params)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want substring %q", err, tt.want)
			}
		})
	}
}

type fakeApproval struct{}

func (fakeApproval) Decide(context.Context, Plan) (ApprovalDecision, error) {
	return ApprovalHold, nil
}

var _ ApprovalPolicy = fakeApproval{}

func feasibleParams() NewPlanParams {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return NewPlanParams{
		ID:               "plan-1",
		ObjectiveID:      "obj-1",
		TenantID:         "tenant-1",
		PolicyID:         "policy-1",
		PolicyVersion:    3,
		ResourceRevision: "rev-1",
		PlannerID:        "immediate",
		PlannerVersion:   "v1",
		Status:           StatusReady,
		Feasibility:      FeasibilityFeasible,
		Window:           objective.TimeWindow{Start: start, End: start.Add(time.Hour)},
		GeneratedAt:      start,
		Steps: []PlanStep{{
			ID:        "step-1",
			Ordinal:   1,
			ExecuteAt: start,
			Status:    StatusReady,
			Version:   1,
			Commands: []PlannedCommand{{
				ID:              "cmd-1",
				CUCode:          "cu-1",
				MetricID:        contracts.MetricElectricalActivePowerSetpoint,
				Value:           model.FloatCommandValue(-40),
				BindingRevision: 4,
			}},
		}},
	}
}
