package port

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// CommandRepository handles persistence for ControlCommand entities.
//
// Write frequency: very high.
// Every Kafka callback (CommandCompletedEvent) triggers exactly one Update call.
// This is the hottest write path in the entire Dispatch service.
type CommandRepository interface {
	// Update persists the runtime fields of a single ControlCommand:
	//   status, retry_count, sent_at, deadline_at, finished_at, result
	// Does NOT touch the parent DispatchAction or DispatchTask.
	Update(ctx context.Context, cmd *model.ControlCommand) error

	// FindExpiredSending returns lightweight ControlCommand objects (ID, ActionID,
	// TenantID, Status, DeadlineAt) for commands currently in Sending state whose
	// deadline has passed. Used exclusively by the TimeoutScanner.
	//
	// The caller uses the returned CommandID to load the full Task tree via
	// TaskRepository.FindByCommandID before running domain logic.
	FindExpiredSending(ctx context.Context, before time.Time) ([]*model.ControlCommand, error)
}
