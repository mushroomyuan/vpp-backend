// Package approvalstub is the ApprovalPolicy used until an approval flow exists.
// Automatic SOC plans do not call it.
package approvalstub

import (
	"context"
	"errors"

	"github.com/mushroomyuan/vpp-backend/decision/domain/plan"
)

// ErrNotImplemented is returned by every call.
var ErrNotImplemented = errors.New("approval_stub: approval is not connected")

// Stub always fails. There is nothing to configure.
type Stub struct{}

// New returns the disconnected approval policy.
func New() *Stub { return &Stub{} }

var _ plan.ApprovalPolicy = (*Stub)(nil)

// Decide returns ErrNotImplemented.
func (s *Stub) Decide(context.Context, plan.Plan) (plan.ApprovalDecision, error) {
	return "", ErrNotImplemented
}
