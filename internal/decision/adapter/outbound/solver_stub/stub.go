// Package solverstub is the Solver used until a planner opts into mathematical optimization.
// The SOC path does not call it.
package solverstub

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/decision/domain/allocation"
)

// ErrNotImplemented is returned by every call.
var ErrNotImplemented = errors.New("solver_stub: solver is not connected")

// Stub always fails. There is nothing to configure.
type Stub struct{}

// New returns the disconnected solver.
func New() *Stub { return &Stub{} }

var _ allocation.Solver = (*Stub)(nil)

// Solve returns ErrNotImplemented.
func (s *Stub) Solve(context.Context, allocation.Problem) (allocation.Solution, error) {
	return allocation.Solution{}, ErrNotImplemented
}
