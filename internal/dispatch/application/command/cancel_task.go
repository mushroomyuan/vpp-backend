package command

import (
	"context"
	"fmt"
	"strings"

	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
	"github.com/mushroomyuan/vpp-backend/platform/decorator"
)

// CancelTask requests cancellation of a non-terminal DispatchTask.
//
// Pending Actions/Commands are cancelled immediately. A Command already
// Sending cannot be recalled from Gateway; the Task is still marked
// Cancelled, and the Sending command's eventual async result is dropped as a
// no-op by HandleCommandResult/TimeoutScanner once the task is terminal.
type CancelTask struct {
	TenantID string
	TaskID   string
}

type CancelTaskResult struct {
	Status model.TaskStatus
}

type CancelTaskHandler = decorator.CommandHandler[CancelTask, *CancelTaskResult]

type cancelTaskHandler struct {
	helper  *dispatchHelper
	metrics decorator.MetricsClient
}

func NewCancelTaskHandler(
	taskRepo port.TaskRepository,
	actionRepo port.ActionRepository,
	commandRepo port.CommandRepository,
	gateway appport.GatewayPort,
	publisher port.TaskEventPublisher,
	dispatcher *service.Dispatcher,
	metricsClient decorator.MetricsClient,
) CancelTaskHandler {
	if taskRepo == nil {
		panic("NewCancelTaskHandler: taskRepo is required")
	}
	if actionRepo == nil {
		panic("NewCancelTaskHandler: actionRepo is required")
	}
	if commandRepo == nil {
		panic("NewCancelTaskHandler: commandRepo is required")
	}
	if gateway == nil {
		panic("NewCancelTaskHandler: gateway is required")
	}
	if publisher == nil {
		panic("NewCancelTaskHandler: publisher is required")
	}
	if dispatcher == nil {
		panic("NewCancelTaskHandler: dispatcher is required")
	}
	return decorator.ApplyCommandDecorators[CancelTask, *CancelTaskResult](
		cancelTaskHandler{
			helper: newDispatchHelper(
				taskRepo, actionRepo, commandRepo, gateway, publisher, dispatcher,
			),
			metrics: metricsClient,
		},
		metricsClient,
	)
}

func (h cancelTaskHandler) Handle(ctx context.Context, cmd CancelTask) (*CancelTaskResult, error) {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(cmd.TaskID) == "" {
		return nil, fmt.Errorf("task_id is required")
	}

	task, err := h.helper.taskRepo.FindByID(ctx, cmd.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil || task.TenantID != cmd.TenantID {
		return nil, domain.ErrTaskNotFound
	}

	outcome, err := h.helper.dispatcher.Cancel(task)
	if err != nil {
		return nil, err
	}
	if err := h.helper.persistOutcome(ctx, task, outcome); err != nil {
		return nil, err
	}

	return &CancelTaskResult{Status: task.Status}, nil
}
