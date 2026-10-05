// Package allocation is where a planner splits a scope target across CUs.
// Solver is the mathematical option. Its problem shape is intentionally unspecified.
package allocation

import "context"

// Problem is the solver input. It has no fields until an optimization contract exists.
type Problem struct{}

// Solution is the solver output paired with Problem.
type Solution struct{}

// Solver is the optional optimizer a planner may select instead of a heuristic.
// The SOC path leaves it uncalled.
type Solver interface {
	Solve(ctx context.Context, problem Problem) (Solution, error)
}
