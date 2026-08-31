package service

import (
	"fmt"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// Dispatcher orchestrates the state machine for DispatchTask → DispatchAction → ControlCommand.
//
// It is stateless and performs no I/O. All changes are made in-memory to the
// provided Task aggregate. The Application layer is responsible for persisting
// the results using the three-tiered Repository model (TaskRepository,
// ActionRepository, CommandRepository) based on the returned CommandResultOutcome.
type Dispatcher struct{}

// NewDispatcher constructs a Dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{}
}

// CommandResultOutcome encapsulates all state changes produced by OnCommandResult
// or OnCommandTimeout. The Application layer uses it to drive precise, fine-grained
// Repository updates instead of rewriting the entire aggregate.
type CommandResultOutcome struct {
	// ChangedCommands lists every ControlCommand whose status changed during this
	// call. The Application layer calls commandRepo.Update for each entry.
	ChangedCommands []*model.ControlCommand

	// ChangedActions lists every DispatchAction whose status changed during this
	// call. The Application layer calls actionRepo.Update for each entry.
	ChangedActions []*model.DispatchAction

	// TaskChanged indicates the Task's own status (or timestamps) changed.
	// The Application layer calls taskRepo.Update when this is true.
	TaskChanged bool

	// NextCommands holds commands that must be dispatched to Gateway immediately
	// after this outcome is processed. In the Sequential path this is the next
	// Pending command; in the retry path it is the retried command itself.
	// Empty on the failure (circuit-breaker) path.
	NextCommands []*model.ControlCommand

	// TaskFinished indicates the task has reached a terminal state (Completed or
	// Failed). The Application layer publishes the corresponding domain event.
	TaskFinished bool
}

// OnCommandResult processes the outcome of a dispatched command and advances the
// state machine for the Command, its parent Action, and the Task.
//
// Success path:
//  1. Marks the Command Succeeded.
//  2. Checks for the next command to dispatch (Sequential continuation).
//  3. If the Action is fully finished, marks it Completed and advances to the
//     next Pending Action, or marks the Task Completed if no more actions remain.
//
// Failure path (FailFast policy, v1 only):
//  1. Marks the Command Failed.
//  2. Applies the circuit breaker: cancels all remaining Pending commands and
//     actions, then marks the Task Failed.
func (d *Dispatcher) OnCommandResult(
	task *model.DispatchTask,
	commandID string,
	result *model.CommandResult,
) (*CommandResultOutcome, error) {
	action, cmd := task.FindCommand(commandID)
	if cmd == nil {
		return nil, fmt.Errorf("%w: %s in task %s", domain.ErrCommandNotFound, commandID, task.ID)
	}

	outcome := &CommandResultOutcome{}

	if result.Success {
		if err := cmd.MarkSucceeded(result); err != nil {
			return nil, err
		}
		outcome.ChangedCommands = append(outcome.ChangedCommands, cmd)
		return d.advanceAfterSuccess(task, action, outcome)
	}

	// Failure path: mark the command failed, then apply the circuit breaker.
	if err := cmd.MarkFailed(result); err != nil {
		return nil, err
	}
	outcome.ChangedCommands = append(outcome.ChangedCommands, cmd)
	return d.applyCircuitBreaker(task, action, outcome)
}

// OnCommandTimeout handles a command whose deadline has passed.
//
// If retries remain (RetryCount < MaxRetries): resets the command to Pending,
// increments RetryCount, and returns it in outcome.NextCommands so the
// Application layer re-dispatches it.
//
// If no retries remain: treats the timeout as a failure and applies the
// FailFast circuit breaker (same path as OnCommandResult with success=false).
func (d *Dispatcher) OnCommandTimeout(
	task *model.DispatchTask,
	commandID string,
) (*CommandResultOutcome, error) {
	action, cmd := task.FindCommand(commandID)
	if cmd == nil {
		return nil, fmt.Errorf("%w: %s in task %s", domain.ErrCommandNotFound, commandID, task.ID)
	}

	// Idempotency guard: only process if the command is still awaiting a result.
	if cmd.Status != model.CommandStatusSending {
		return &CommandResultOutcome{}, nil
	}

	if err := cmd.MarkTimeout(); err != nil {
		return nil, err
	}

	if cmd.CanRetry() {
		if err := cmd.ResetForRetry(); err != nil {
			return nil, err
		}
		return &CommandResultOutcome{
			ChangedCommands: []*model.ControlCommand{cmd},
			NextCommands:    []*model.ControlCommand{cmd},
		}, nil
	}

	// Retries exhausted: convert to a failure and circuit-break.
	failResult := model.NewFailureResult(
		"timeout",
		fmt.Sprintf("command timed out after %d attempt(s)", cmd.RetryCount+1),
	)
	if err := cmd.MarkFailed(failResult); err != nil {
		return nil, err
	}
	outcome := &CommandResultOutcome{
		ChangedCommands: []*model.ControlCommand{cmd},
	}
	return d.applyCircuitBreaker(task, action, outcome)
}

// Cancel processes an externally requested cancellation of the task (e.g. the
// CancelTask RPC). It is the "graceful" counterpart to applyCircuitBreaker:
// instead of marking everything Failed after an error, it marks every
// non-terminal Action Cancelled (via CancelPendingCommands + Action.Cancel)
// and finally the Task itself.
//
// Commands already Sending are left untouched — Dispatch has no RPC to recall
// a command already delivered to Gateway, so its eventual async result (or
// timeout) must still be handled by the caller. The Application layer treats
// that as a no-op once the owning Task is terminal (see the task.IsFinished
// guards in HandleCommandResult / TimeoutScanner), which keeps this method
// safe to call concurrently with an in-flight command's callback.
//
// Returns domain.ErrTaskAlreadyDone if the task has already reached a
// terminal state (Completed, Failed, or Cancelled).
func (d *Dispatcher) Cancel(task *model.DispatchTask) (*CommandResultOutcome, error) {
	if task.IsFinished() {
		return nil, domain.ErrTaskAlreadyDone
	}

	outcome := &CommandResultOutcome{}
	for _, a := range task.Actions {
		if a.Status != model.ActionStatusPending && a.Status != model.ActionStatusRunning {
			continue
		}
		a.CancelPendingCommands()
		for _, cmd := range a.Commands {
			if cmd.Status == model.CommandStatusCancelled {
				outcome.ChangedCommands = append(outcome.ChangedCommands, cmd)
			}
		}
		if err := a.Cancel(); err != nil {
			return nil, fmt.Errorf("dispatch: cancel action %s: %w", a.ID, err)
		}
		outcome.ChangedActions = append(outcome.ChangedActions, a)
	}

	if err := task.Cancel(); err != nil {
		return nil, fmt.Errorf("dispatch: cancel task %s: %w", task.ID, err)
	}
	outcome.TaskChanged = true
	outcome.TaskFinished = true
	return outcome, nil
}

// advanceAfterSuccess is called when a command succeeds. It drives the
// sequential continuation and action/task completion logic.
func (d *Dispatcher) advanceAfterSuccess(
	task *model.DispatchTask,
	action *model.DispatchAction,
	outcome *CommandResultOutcome,
) (*CommandResultOutcome, error) {
	// Check whether this action has more commands to dispatch right now.
	nextCmds := action.CommandsToDispatch()
	if len(nextCmds) > 0 {
		// Sequential: at least one more command to send, or
		// Parallel: more pending commands exist.
		outcome.NextCommands = nextCmds
		return outcome, nil
	}

	// No commands to dispatch at this moment.
	// For Parallel actions, some commands may still be in-flight (Sending).
	if !action.AllCommandsFinished() {
		return outcome, nil
	}

	// All commands in this action have reached a terminal state.
	// (AnyCommandFailed should be false on the success path, but guard defensively.)
	if action.AnyCommandFailed() {
		return d.applyCircuitBreaker(task, action, outcome)
	}

	if err := action.Complete(); err != nil {
		return nil, fmt.Errorf("dispatch: complete action %s: %w", action.ID, err)
	}
	outcome.ChangedActions = append(outcome.ChangedActions, action)

	// Advance to the next Pending Action if one exists.
	next := task.NextPendingAction()
	if next != nil {
		if err := next.Start(); err != nil {
			return nil, fmt.Errorf("dispatch: start next action %s: %w", next.ID, err)
		}
		outcome.ChangedActions = append(outcome.ChangedActions, next)
		outcome.NextCommands = next.CommandsToDispatch()
		return outcome, nil
	}

	// All actions are done — the task is complete.
	if err := task.Complete(); err != nil {
		return nil, fmt.Errorf("dispatch: complete task %s: %w", task.ID, err)
	}
	outcome.TaskChanged = true
	outcome.TaskFinished = true
	return outcome, nil
}

// applyCircuitBreaker implements the FailFast interruption policy:
//  1. Cancels all Pending commands in the failing action.
//  2. Marks the failing action as Failed.
//  3. Cancels all subsequent Pending actions (and their Pending commands).
//  4. Marks the task as Failed.
//
// The failing command must already be in Failed state before this is called.
func (d *Dispatcher) applyCircuitBreaker(
	task *model.DispatchTask,
	failedAction *model.DispatchAction,
	outcome *CommandResultOutcome,
) (*CommandResultOutcome, error) {
	// 1. Cancel all Pending commands in the failing action.
	for _, cmd := range failedAction.Commands {
		if cmd.Status == model.CommandStatusPending {
			_ = cmd.Cancel() // always valid for Pending commands
			outcome.ChangedCommands = append(outcome.ChangedCommands, cmd)
		}
	}

	// 2. Fail the current action (must be Running at this point).
	if err := failedAction.Fail(); err != nil {
		return nil, fmt.Errorf("dispatch: circuit breaker: fail action %s: %w", failedAction.ID, err)
	}
	outcome.ChangedActions = append(outcome.ChangedActions, failedAction)

	// 3. Cancel all subsequent Pending actions and their Pending commands.
	for _, a := range task.Actions {
		if a.ID == failedAction.ID || a.Status != model.ActionStatusPending {
			continue
		}
		for _, cmd := range a.Commands {
			if cmd.Status == model.CommandStatusPending {
				_ = cmd.Cancel()
				outcome.ChangedCommands = append(outcome.ChangedCommands, cmd)
			}
		}
		_ = a.Cancel() // Pending → Cancelled, always valid
		outcome.ChangedActions = append(outcome.ChangedActions, a)
	}

	// 4. Fail the task.
	if err := task.Fail(); err != nil {
		return nil, fmt.Errorf("dispatch: circuit breaker: fail task %s: %w", task.ID, err)
	}
	outcome.TaskChanged = true
	outcome.TaskFinished = true
	return outcome, nil
}
