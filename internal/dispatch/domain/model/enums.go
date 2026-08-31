package model

// TaskType categorises the business intent of a DispatchTask.
type TaskType string

const (
	TaskTypeControl TaskType = "control"
)

// TriggerType describes how a DispatchTask was initiated.
type TriggerType string

const (
	TriggerManual    TriggerType = "manual"
	TriggerScheduled TriggerType = "scheduled"
	TriggerAutomatic TriggerType = "automatic"
)

// FailurePolicy determines the Dispatcher's behaviour when a Command or Action fails.
// v1 supports FailFast only. Additional policies are reserved for future Saga-style use.
type FailurePolicy string

const (
	// FailFast immediately halts the task: all remaining Pending Actions and Commands
	// are Cancelled, and the Task is marked Failed. No automatic compensation is performed.
	FailFast FailurePolicy = "fail_fast"

	// Future (not implemented in v1):
	//   Compensate        FailurePolicy = "compensate"        // auto reverse-control
	//   ManualIntervention FailurePolicy = "manual_intervention" // pause and await human
)

// TaskStatus is the lifecycle state of a DispatchTask.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// ActionType categorises the business intent of a DispatchAction.
type ActionType string

const (
	ActionTypeControl ActionType = "control"
)

// ExecutionPolicy controls how Commands within an Action are dispatched.
type ExecutionPolicy string

const (
	// Sequential sends commands one at a time, waiting for each to reach a
	// terminal state before sending the next.
	Sequential ExecutionPolicy = "sequential"

	// Parallel sends all commands concurrently and waits for all to finish.
	Parallel ExecutionPolicy = "parallel"
)

// ActionStatus is the lifecycle state of a DispatchAction.
type ActionStatus string

const (
	ActionStatusPending   ActionStatus = "pending"
	ActionStatusRunning   ActionStatus = "running"
	ActionStatusCompleted ActionStatus = "completed"
	ActionStatusFailed    ActionStatus = "failed"
	// ActionStatusCancelled is set either by the circuit-breaker (a preceding
	// Action failed: all subsequent Pending Actions are cancelled without
	// executing) or by an externally requested CancelTask.
	ActionStatusCancelled ActionStatus = "cancelled"
)

// CommandStatus is the lifecycle state of a ControlCommand.
type CommandStatus string

const (
	CommandStatusPending   CommandStatus = "pending"
	CommandStatusSending   CommandStatus = "sending"
	CommandStatusSucceeded CommandStatus = "succeeded"
	CommandStatusFailed    CommandStatus = "failed"
	// CommandStatusTimeout means the Gateway did not respond within the deadline.
	// It is not a terminal state: if CanRetry() is true, the command is reset to Pending.
	CommandStatusTimeout CommandStatus = "timeout"
	// CommandStatusCancelled is set on Pending commands that were never
	// dispatched — either because an earlier command/action failed
	// (circuit-breaker) or because the task was cancelled externally
	// (CancelTask). Commands already Sending are never force-cancelled; see
	// Dispatcher.Cancel.
	CommandStatusCancelled CommandStatus = "cancelled"
)
