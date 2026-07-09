package command

import (
	"context"
	"fmt"
	"time"

	appport "github.com/mushroomyuan/vpp-backend/dispatch/application/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/service"
)

// dispatchHelper holds shared dependencies for gateway dispatch and fine-grained
// persistence used by SubmitTask, HandleCommandResult, and TimeoutScanner.
type dispatchHelper struct {
	taskRepo    port.TaskRepository
	actionRepo  port.ActionRepository
	commandRepo port.CommandRepository
	gateway     appport.GatewayPort
	publisher   port.TaskEventPublisher
	dispatcher  *service.Dispatcher
}

func newDispatchHelper(
	taskRepo port.TaskRepository,
	actionRepo port.ActionRepository,
	commandRepo port.CommandRepository,
	gateway appport.GatewayPort,
	publisher port.TaskEventPublisher,
	dispatcher *service.Dispatcher,
) *dispatchHelper {
	return &dispatchHelper{
		taskRepo:    taskRepo,
		actionRepo:  actionRepo,
		commandRepo: commandRepo,
		gateway:     gateway,
		publisher:   publisher,
		dispatcher:  dispatcher,
	}
}

// persistOutcome writes only the rows that changed according to outcome, then
// publishes a terminal task event when TaskFinished is set.
func (h *dispatchHelper) persistOutcome(
	ctx context.Context,
	task *model.DispatchTask,
	outcome *service.CommandResultOutcome,
) error {
	if outcome == nil {
		return nil
	}
	for _, cmd := range outcome.ChangedCommands {
		if err := h.commandRepo.Update(ctx, cmd); err != nil {
			return fmt.Errorf("persist command %s: %w", cmd.ID, err)
		}
	}
	for _, action := range outcome.ChangedActions {
		if err := h.actionRepo.Update(ctx, action); err != nil {
			return fmt.Errorf("persist action %s: %w", action.ID, err)
		}
	}
	if outcome.TaskChanged {
		if err := h.taskRepo.Update(ctx, task); err != nil {
			return fmt.Errorf("persist task %s: %w", task.ID, err)
		}
	}
	if outcome.TaskFinished {
		switch task.Status {
		case model.TaskStatusCompleted:
			if err := h.publisher.PublishTaskCompleted(ctx, task); err != nil {
				return fmt.Errorf("publish task completed: %w", err)
			}
		case model.TaskStatusFailed:
			if err := h.publisher.PublishTaskFailed(ctx, task); err != nil {
				return fmt.Errorf("publish task failed: %w", err)
			}
		}
	}
	return nil
}

// startTaskAndFirstAction transitions the task and its first pending action to
// Running and persists those status changes. Returns the commands that should
// be dispatched immediately.
func (h *dispatchHelper) startTaskAndFirstAction(
	ctx context.Context,
	task *model.DispatchTask,
) ([]*model.ControlCommand, error) {
	if err := task.Start(); err != nil {
		return nil, err
	}
	if err := h.taskRepo.Update(ctx, task); err != nil {
		return nil, fmt.Errorf("persist task start: %w", err)
	}
	if err := h.publisher.PublishTaskStarted(ctx, task); err != nil {
		return nil, fmt.Errorf("publish task started: %w", err)
	}

	action := task.NextPendingAction()
	if action == nil {
		return nil, fmt.Errorf("dispatch: task %s has no pending action to start", task.ID)
	}
	if err := action.Start(); err != nil {
		return nil, err
	}
	if err := h.actionRepo.Update(ctx, action); err != nil {
		return nil, fmt.Errorf("persist action start: %w", err)
	}
	return action.CommandsToDispatch(), nil
}

// dispatchCommands sends each command to Gateway and advances domain state
// according to the acceptance status. Continuations (NextCommands) are processed
// iteratively until the batch is drained or the task finishes.
func (h *dispatchHelper) dispatchCommands(
	ctx context.Context,
	task *model.DispatchTask,
	cmds []*model.ControlCommand,
) error {
	queue := append([]*model.ControlCommand(nil), cmds...)
	for len(queue) > 0 {
		if task.IsFinished() {
			return nil
		}
		cmd := queue[0]
		queue = queue[1:]

		next, stop, err := h.dispatchOne(ctx, task, cmd)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		queue = append(queue, next...)
	}
	return nil
}

// dispatchOne executes a single command against Gateway and returns any
// continuation commands. stop=true means the task has finished (circuit break
// or completion) and the caller should not process further queued commands.
func (h *dispatchHelper) dispatchOne(
	ctx context.Context,
	task *model.DispatchTask,
	cmd *model.ControlCommand,
) (next []*model.ControlCommand, stop bool, err error) {
	if cmd.Status != model.CommandStatusPending {
		return nil, false, nil
	}

	gwResult, gwErr := h.gateway.ExecuteCommand(ctx, cmd)
	if gwErr != nil {
		return h.applyFailure(ctx, task, cmd, "gateway_error", gwErr.Error())
	}
	if gwResult == nil {
		return h.applyFailure(ctx, task, cmd, "gateway_error", "nil gateway result")
	}

	switch gwResult.Status {
	case appport.GatewayAccepted:
		now := time.Now()
		if err := cmd.MarkSending(now); err != nil {
			return nil, false, err
		}
		if err := h.commandRepo.Update(ctx, cmd); err != nil {
			return nil, false, fmt.Errorf("persist command sending %s: %w", cmd.ID, err)
		}
		return nil, false, nil

	case appport.GatewayCompleted:
		now := time.Now()
		if err := cmd.MarkSending(now); err != nil {
			return nil, false, err
		}
		var result *model.CommandResult
		if gwResult.Success {
			result = model.NewSuccessResult(now)
		} else {
			msg := gwResult.Message
			if msg == "" {
				msg = "gateway completed with failure"
			}
			result = model.NewFailureResult("gateway_completed_failure", msg)
		}
		outcome, err := h.dispatcher.OnCommandResult(task, cmd.ID, result)
		if err != nil {
			return nil, false, err
		}
		if err := h.persistOutcome(ctx, task, outcome); err != nil {
			return nil, false, err
		}
		return outcome.NextCommands, outcome.TaskFinished, nil

	case appport.GatewayRejected:
		msg := gwResult.Message
		if msg == "" {
			msg = "gateway rejected command"
		}
		return h.applyFailure(ctx, task, cmd, "gateway_rejected", msg)

	default:
		return h.applyFailure(ctx, task, cmd, "gateway_error",
			fmt.Sprintf("unknown gateway status %d", gwResult.Status))
	}
}

func (h *dispatchHelper) applyFailure(
	ctx context.Context,
	task *model.DispatchTask,
	cmd *model.ControlCommand,
	errorCode, errorMessage string,
) (next []*model.ControlCommand, stop bool, err error) {
	result := model.NewFailureResult(errorCode, errorMessage)
	outcome, err := h.dispatcher.OnCommandResult(task, cmd.ID, result)
	if err != nil {
		return nil, false, err
	}
	if err := h.persistOutcome(ctx, task, outcome); err != nil {
		return nil, false, err
	}
	return outcome.NextCommands, outcome.TaskFinished, nil
}

// applyOutcomeAndContinue persists an outcome produced by the dispatcher and
// dispatches any NextCommands.
func (h *dispatchHelper) applyOutcomeAndContinue(
	ctx context.Context,
	task *model.DispatchTask,
	outcome *service.CommandResultOutcome,
) error {
	if err := h.persistOutcome(ctx, task, outcome); err != nil {
		return err
	}
	if len(outcome.NextCommands) == 0 {
		return nil
	}
	return h.dispatchCommands(ctx, task, outcome.NextCommands)
}
