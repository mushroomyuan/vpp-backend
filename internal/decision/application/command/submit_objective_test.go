package command

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/decision/domain/port"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

func TestSubmitObjectiveHandler_RejectsMissingObjective(t *testing.T) {
	handler := NewSubmitObjectiveHandler(fixedPlanner{}, &memoryPlans{}, nil, nopMetrics{})
	_, err := handler.Handle(context.Background(), SubmitObjective{})
	if err == nil || !strings.Contains(err.Error(), "objective is required") {
		t.Fatalf("err = %v", err)
	}
}

type countingApproval struct{ calls int }

func (c *countingApproval) Decide(context.Context, plan.Plan) (plan.ApprovalDecision, error) {
	c.calls++
	return plan.ApprovalHold, errors.New("approval was called")
}

type fixedPlanner struct{ planned *plan.Plan }

func (f fixedPlanner) ID() string      { return "immediate" }
func (f fixedPlanner) Version() string { return "v1" }
func (f fixedPlanner) Plan(context.Context, objective.Objective, dctx.DecisionContext) (*plan.Plan, error) {
	return f.planned, nil
}

type memoryPlans struct{ saved *plan.Plan }

func (m *memoryPlans) Save(_ context.Context, in plan.SaveInput) error {
	m.saved = in.Plan
	return nil
}
func (m *memoryPlans) ClaimDue(context.Context, string, time.Time, time.Duration) (plan.Claim, error) {
	return plan.Claim{}, plan.ErrNoneDue
}
func (m *memoryPlans) MarkSubmitted(context.Context, plan.Claim, string, time.Time) error {
	return nil
}
func (m *memoryPlans) MarkUncertain(context.Context, plan.Claim, string, time.Time, time.Time) error {
	return nil
}
func (m *memoryPlans) MarkStale(context.Context, plan.Claim, string, time.Time) error { return nil }
func (m *memoryPlans) MarkFailed(context.Context, plan.Claim, string, time.Time) error {
	return nil
}

func TestSubmitObjectiveHandler_StoresReadyPlanWithoutApproval(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	obj, err := objective.NewPowerObjective(objective.NewPowerObjectiveParams{
		ID: "22222222-2222-4222-8222-222222222222", TenantID: "tenant-1",
		Scope:    policy.TargetScope{Type: port.ScopeCU, ID: "cu-1"},
		MetricID: contracts.MetricElectricalActivePowerSetpoint, TargetPowerKW: -40,
		Window: objective.TimeWindow{Start: now, End: now.Add(time.Hour)},
		Source: objective.SourcePolicy, SourceID: "11111111-1111-4111-8111-111111111111",
		IdempotencyKey: "obj-1", PolicyID: "11111111-1111-4111-8111-111111111111", PolicyVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := plan.NewPlan(plan.NewPlanParams{
		ID: obj.ID, ObjectiveID: obj.ID, TenantID: obj.TenantID, PolicyID: obj.PolicyID, PolicyVersion: 1,
		ResourceRevision: "rev-1", PlannerID: "immediate", PlannerVersion: "v1",
		Status: plan.StatusReady, Feasibility: plan.FeasibilityFeasible,
		Window: obj.Window, GeneratedAt: now,
		Steps: []plan.PlanStep{{
			ID: "44444444-4444-4444-8444-444444444444", Ordinal: 1, ExecuteAt: now,
			Status: plan.StatusReady, Version: 1,
			Commands: []plan.PlannedCommand{{
				ID: "55555555-5555-4555-8555-555555555555", CUCode: "cu-1",
				MetricID: contracts.MetricElectricalActivePowerSetpoint,
				Value:    model.FloatCommandValue(-40), BindingRevision: 7,
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	approval := &countingApproval{}
	store := &memoryPlans{}
	handler := NewSubmitObjectiveHandler(fixedPlanner{planned: ready}, store, approval, nopMetrics{})
	saved, err := handler.Handle(context.Background(), SubmitObjective{
		Objective: obj, Direction: policy.DirectionCharge, Now: now, CooldownUntil: now.Add(time.Minute),
	})
	if err != nil || saved.Status != plan.StatusReady || store.saved == nil || approval.calls != 0 {
		t.Fatalf("saved=%v err=%v calls=%d", saved, err, approval.calls)
	}
}

type nopMetrics struct{}

func (nopMetrics) Count(string, string, string)           {}
func (nopMetrics) CountN(string, string, string, float64) {}
func (nopMetrics) Observe(string, string, time.Duration)  {}
func (nopMetrics) TrackInFlight(string, string) func()    { return func() {} }

var _ decorator.MetricsClient = nopMetrics{}
