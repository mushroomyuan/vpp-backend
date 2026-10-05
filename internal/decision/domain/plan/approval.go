package plan

import "context"

// ApprovalDecision is the outcome of an approval policy.
type ApprovalDecision string

const (
	ApprovalApprove ApprovalDecision = "approve"
	ApprovalReject  ApprovalDecision = "reject"
	ApprovalHold    ApprovalDecision = "hold"
)

// ApprovalPolicy decides whether a plan may proceed.
// Automatic SOC plans leave this uncalled. The only implementation returns an error.
type ApprovalPolicy interface {
	Decide(ctx context.Context, candidate Plan) (ApprovalDecision, error)
}
