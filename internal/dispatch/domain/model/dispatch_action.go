package model

import "fmt"

// DispatchAction groups one or more ControlCommands into a named business operation.
//
// Within a Task, Actions are executed in ascending Sequence order.
// Commands within an Action are executed according to ExecutionPolicy:
//   - Sequential: one command at a time, next dispatched only after previous succeeds.
//   - Parallel:   all commands dispatched concurrently; Action completes when all finish.
type DispatchAction struct {
	ID       string
	TaskID   string
	TenantID string

	Name     string
	Type     ActionType
	Sequence int

	Status          ActionStatus
	ExecutionPolicy ExecutionPolicy

	// Commands are stored in dispatch order. For Sequential actions, the slice
	// order defines the execution order (first Pending command in slice is sent first).
	Commands []*ControlCommand
}

// Start transitions the action from Pending to Running.
func (a *DispatchAction) Start() error {
	if a.Status != ActionStatusPending {
		return fmt.Errorf("dispatch: action %s cannot start from %q", a.ID, a.Status)
	}
	a.Status = ActionStatusRunning
	return nil
}

// Complete transitions the action from Running to Completed.
func (a *DispatchAction) Complete() error {
	if a.Status != ActionStatusRunning {
		return fmt.Errorf("dispatch: action %s cannot complete from %q", a.ID, a.Status)
	}
	a.Status = ActionStatusCompleted
	return nil
}

// Fail transitions the action from Running to Failed.
func (a *DispatchAction) Fail() error {
	if a.Status != ActionStatusRunning {
		return fmt.Errorf("dispatch: action %s cannot fail from %q", a.ID, a.Status)
	}
	a.Status = ActionStatusFailed
	return nil
}

// Cancel transitions a Pending or Running action to Cancelled.
// Used by the circuit-breaker in Dispatcher when a preceding action fails.
func (a *DispatchAction) Cancel() error {
	if a.Status != ActionStatusPending && a.Status != ActionStatusRunning {
		return fmt.Errorf("dispatch: action %s cannot be cancelled from %q", a.ID, a.Status)
	}
	a.Status = ActionStatusCancelled
	return nil
}

// CancelPendingCommands sets every Pending command in this action to Cancelled.
// Commands that are already Sending, Succeeded, Failed, Timeout, or Cancelled
// are left unchanged.
func (a *DispatchAction) CancelPendingCommands() {
	for _, cmd := range a.Commands {
		if cmd.Status == CommandStatusPending {
			cmd.Status = CommandStatusCancelled
		}
	}
}

// CommandsToDispatch returns the commands that should be sent to Gateway right now.
//
// Sequential policy:
//
//	Returns the first Pending command (in slice order), but only if no command
//	is currently in Sending state. If a command is already in-flight, returns nil
//	(the continuation will be driven by the Kafka callback for that command).
//
// Parallel policy:
//
//	Returns all Pending commands regardless of any in-flight commands.
func (a *DispatchAction) CommandsToDispatch() []*ControlCommand {
	switch a.ExecutionPolicy {
	case Parallel:
		var cmds []*ControlCommand
		for _, cmd := range a.Commands {
			if cmd.Status == CommandStatusPending {
				cmds = append(cmds, cmd)
			}
		}
		return cmds

	default: // Sequential
		// Do not dispatch if another command is already in-flight.
		for _, cmd := range a.Commands {
			if cmd.Status == CommandStatusSending {
				return nil
			}
		}
		// Return the first Pending command in slice (dispatch) order.
		for _, cmd := range a.Commands {
			if cmd.Status == CommandStatusPending {
				return []*ControlCommand{cmd}
			}
		}
		return nil
	}
}

// AllCommandsFinished reports whether every command has reached a terminal state
// (Succeeded, Failed, or Cancelled). Timeout and Sending are not terminal.
func (a *DispatchAction) AllCommandsFinished() bool {
	for _, cmd := range a.Commands {
		if !cmd.IsTerminal() {
			return false
		}
	}
	return true
}

// AnyCommandFailed reports whether any command in this action has Failed.
// Used to determine whether the Action should be marked Failed vs Completed.
func (a *DispatchAction) AnyCommandFailed() bool {
	for _, cmd := range a.Commands {
		if cmd.Status == CommandStatusFailed {
			return true
		}
	}
	return false
}
