package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// TaskEventPublisher publishes domain events for DispatchTask lifecycle transitions.
//
// Downstream consumers (e.g., an alert service) subscribe to these events to
// trigger operator notifications when a task fails mid-execution (FailFast policy).
type TaskEventPublisher interface {
	PublishTaskStarted(ctx context.Context, task *model.DispatchTask) error
	PublishTaskCompleted(ctx context.Context, task *model.DispatchTask) error
	PublishTaskFailed(ctx context.Context, task *model.DispatchTask) error
	PublishTaskCancelled(ctx context.Context, task *model.DispatchTask) error
}
