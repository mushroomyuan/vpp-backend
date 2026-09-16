// Package port holds outbound interfaces consumed by Optimization's
// application layer (as opposed to domain/port, consumed by
// domain/service). This split mirrors dispatch's own precedent:
// dispatch/domain/port holds its repository interfaces (consumed by
// domain services), while dispatch/application/port holds GatewayPort
// (consumed by application/command, since only the use-case layer calls
// out to Gateway — domain/service.Dispatcher never does). DispatchPort
// belongs here for the same reason: Allocate (domain/service) only
// produces CommandSpecs, it never submits them — only the application
// layer's decision loop does.
package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/optimization/domain/model"
)

// SubmitResult is DispatchPort's return shape, mirroring
// dispatchpb.SubmitTaskResponse (TaskID + Status) closely enough for the
// application layer to log/observe without depending on the dispatch
// proto package outside the dispatch_grpc adapter.
type SubmitResult struct {
	TaskID string
	Status string
}

// DispatchPort is what the application layer's decision cycle uses to
// submit Allocate's output as a dispatch Task.
//
// Optimization always submits with TriggerType "automatic" — see
// discussion/2026-09-04.md §3.1's leftover TODO about needing a
// Source/initiator field on SubmitTask to distinguish algorithmic from
// human dispatch. The Optimization design plan §8 corrects that: dispatch
// already has a TriggerType field ("manual"/"scheduled"/"automatic")
// that is fully wired end-to-end to Postgres — Optimization setting
// TriggerType="automatic" is sufficient, no new proto field is needed.
type DispatchPort interface {
	// SubmitTask packs commands into one dispatch Task (one parallel
	// ActionSpec — see the design plan §8/dispatch_grpc adapter for why
	// "parallel": Optimization's commands from one decision cycle target
	// independent CUs with no sequencing requirement between them).
	// commands must be non-empty.
	SubmitTask(ctx context.Context, tenantID, name string, commands []model.CommandSpec) (SubmitResult, error)
}
