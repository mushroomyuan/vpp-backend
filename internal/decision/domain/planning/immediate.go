package planning

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
)

const (
	// ImmediatePlannerID is the planner id copied onto SOC plans.
	ImmediatePlannerID = "immediate"
	// ImmediatePlannerVersion is the planner version copied onto SOC plans.
	ImmediatePlannerVersion = "v1"
)

// ImmediatePlanner turns one power objective into a single step that executes
// at the context collection time. It calls the allocator and does not call Forecast or Solver.
type ImmediatePlanner struct {
	allocator allocation.Allocator
	newID     func() string
}

// NewImmediatePlanner requires an allocator and an id function.
func NewImmediatePlanner(allocator allocation.Allocator, newID func() string) *ImmediatePlanner {
	if allocator == nil {
		panic("NewImmediatePlanner: allocator is required")
	}
	if newID == nil {
		panic("NewImmediatePlanner: id function is required")
	}
	return &ImmediatePlanner{allocator: allocator, newID: newID}
}

var _ Planner = (*ImmediatePlanner)(nil)

func (p *ImmediatePlanner) ID() string      { return ImmediatePlannerID }
func (p *ImmediatePlanner) Version() string { return ImmediatePlannerVersion }

// Plan builds one ready step, or a rejected plan when no CU can take power.
func (p *ImmediatePlanner) Plan(ctx context.Context, obj objective.Objective, dc dctx.DecisionContext) (*plan.Plan, error) {
	if p == nil {
		return nil, fmt.Errorf("immediate planner: planner is nil")
	}
	power, ok := obj.(*objective.PowerObjective)
	if !ok || power == nil {
		return nil, fmt.Errorf("immediate planner: power objective is required")
	}
	if err := power.Validate(); err != nil {
		return nil, err
	}
	allocated, err := p.allocator.Allocate(ctx, power, dc)
	if err != nil {
		return nil, err
	}

	params := plan.NewPlanParams{
		ID:               p.newID(),
		ObjectiveID:      power.ID,
		TenantID:         power.TenantID,
		PolicyID:         power.PolicyID,
		PolicyVersion:    power.PolicyVersion,
		ResourceRevision: dc.ResourceRevision(),
		PlannerID:        p.ID(),
		PlannerVersion:   p.Version(),
		Feasibility:      allocated.Feasibility,
		UnmetPowerKW:     allocated.UnmetPowerKW,
		Window:           power.Window,
		GeneratedAt:      dc.CollectedAt,
	}
	switch allocated.Feasibility {
	case plan.FeasibilityFeasible, plan.FeasibilityPartiallyFeasible:
		params.Status = plan.StatusReady
		commands := make([]plan.PlannedCommand, len(allocated.Commands))
		for i, cmd := range allocated.Commands {
			commands[i] = cmd
			commands[i].ID = p.newID()
		}
		params.Steps = []plan.PlanStep{{
			ID:        p.newID(),
			Ordinal:   1,
			ExecuteAt: dc.CollectedAt,
			Status:    plan.StatusReady,
			Version:   1,
			Commands:  commands,
		}}
	case plan.FeasibilityInfeasible:
		params.Status = plan.StatusRejected
	default:
		return nil, fmt.Errorf("immediate planner: unknown feasibility %q", allocated.Feasibility)
	}
	return plan.NewPlan(params)
}
