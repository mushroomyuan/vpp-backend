// Package planning turns an objective and a decision context into a plan.
// Forecast and Solver stay optional dependencies. The SOC path leaves them uncalled.
package planning

import (
	"context"

	dctx "github.com/mushroomyuan/vpp-backend/decision/domain/context"
	"github.com/mushroomyuan/vpp-backend/decision/domain/objective"
	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
)

// Planner chooses an allocator or a solver and returns one plan.
// ID and Version are copied onto that plan. ImmediatePlanner is the SOC implementation.
type Planner interface {
	ID() string
	Version() string
	Plan(ctx context.Context, objective objective.Objective, decision dctx.DecisionContext) (*plan.Plan, error)
}
