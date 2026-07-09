package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// TaskRepository handles persistence for the DispatchTask aggregate root.
//
// Write frequency: very low.
// A task is saved once at creation and updated only when its own status changes
// (start, complete, fail, cancel). Actions and Commands are persisted through
// their own repositories.
type TaskRepository interface {
	// Save persists the complete Task tree (Task + Actions + Commands) in a single
	// operation. Called exactly once per task at creation time.
	Save(ctx context.Context, task *model.DispatchTask) error

	// Update persists only the DispatchTask row itself (status, started_at, finished_at).
	// Does NOT touch dispatch_actions or control_commands rows.
	Update(ctx context.Context, task *model.DispatchTask) error

	// FindByID loads the complete Task tree including all Actions and Commands.
	FindByID(ctx context.Context, id string) (*model.DispatchTask, error)

	// FindByCommandID loads the complete Task tree for the task that owns the
	// given CommandID. Used by HandleCommandResult and TimeoutScanner to
	// reconstruct the in-memory aggregate before running domain logic.
	FindByCommandID(ctx context.Context, commandID string) (*model.DispatchTask, error)
}
