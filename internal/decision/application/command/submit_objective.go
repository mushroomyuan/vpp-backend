package command

import (
	"context"
	"fmt"
	"time"

	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
	"github.com/mushroomyuan/vpp-backend/decision/domain/planning"
	"github.com/mushroomyuan/vpp-backend/decision/domain/policy"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// SubmitObjective is one objective plus the cooldown window claimed with its plan.
type SubmitObjective struct {
	Objective     *objective.PowerObjective
	DC            dctx.DecisionContext
	Direction     policy.Direction
	Now           time.Time
	CooldownUntil time.Time
}

// SubmitObjectiveHandler plans an objective and saves the plan, commands, cooldown, and outbox.
type SubmitObjectiveHandler = decorator.CommandHandler[SubmitObjective, *plan.Plan]

// submitObjectiveHandler plans and saves.
// approval is kept for a later gate and is not called: an automatic policy plan is stored ready.
type submitObjectiveHandler struct {
	planner  planning.Planner
	plans    plan.Repository
	approval plan.ApprovalPolicy
}

// NewSubmitObjectiveHandler plans and saves. approval may be nil until a gate is connected.
func NewSubmitObjectiveHandler(planner planning.Planner, plans plan.Repository, approval plan.ApprovalPolicy, metricsClient decorator.MetricsClient) SubmitObjectiveHandler {
	if planner == nil || plans == nil || metricsClient == nil {
		panic("NewSubmitObjectiveHandler: planner, plan repository, and metrics are required")
	}
	return decorator.ApplyCommandDecorators[SubmitObjective, *plan.Plan](
		submitObjectiveHandler{planner: planner, plans: plans, approval: approval},
		metricsClient,
	)
}

// Handle plans the objective and saves the result.
// A lost cooldown claim returns plan.ErrCooldownHeld and writes nothing.
// ApprovalPolicy is not consulted.
func (h submitObjectiveHandler) Handle(ctx context.Context, cmd SubmitObjective) (*plan.Plan, error) {
	if cmd.Objective == nil {
		return nil, fmt.Errorf("submit_objective: objective is required")
	}
	if err := cmd.Objective.Validate(); err != nil {
		return nil, err
	}
	planned, err := h.planner.Plan(ctx, cmd.Objective, cmd.DC)
	if err != nil {
		return nil, err
	}
	if err := h.plans.Save(ctx, plan.SaveInput{
		Objective:     cmd.Objective,
		Plan:          planned,
		Direction:     cmd.Direction,
		Now:           cmd.Now,
		CooldownUntil: cmd.CooldownUntil,
	}); err != nil {
		return nil, err
	}
	return planned, nil
}
