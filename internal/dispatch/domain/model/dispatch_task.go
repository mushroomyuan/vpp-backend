package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DispatchTask is the aggregate root of the dispatch domain.
//
// A Task orchestrates one or more DispatchActions executed in ascending Sequence
// order. The Task lifecycle progresses through: Pending → Running → Completed|Failed|Cancelled.
//
// Actions are always started sequentially by Sequence number; within a single
// Action, Commands may be dispatched sequentially or in parallel depending on
// the Action's ExecutionPolicy.
type DispatchTask struct {
	ID          string
	TenantID    string
	Name        string
	Description string

	Type          TaskType
	TriggerType   TriggerType
	FailurePolicy FailurePolicy // always FailFast in v1; reserved for future Saga policies

	Status     TaskStatus
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time

	Actions []*DispatchAction
}

// Start transitions the task from Pending to Running.
func (t *DispatchTask) Start() error {
	if t.Status != TaskStatusPending {
		return fmt.Errorf("dispatch: task %s cannot start from %q", t.ID, t.Status)
	}
	now := time.Now()
	t.Status = TaskStatusRunning
	t.StartedAt = &now
	return nil
}

// Complete transitions the task from Running to Completed.
func (t *DispatchTask) Complete() error {
	if t.Status != TaskStatusRunning {
		return fmt.Errorf("dispatch: task %s cannot complete from %q", t.ID, t.Status)
	}
	now := time.Now()
	t.Status = TaskStatusCompleted
	t.FinishedAt = &now
	return nil
}

// Fail transitions the task from Running to Failed.
func (t *DispatchTask) Fail() error {
	if t.Status != TaskStatusRunning {
		return fmt.Errorf("dispatch: task %s cannot fail from %q", t.ID, t.Status)
	}
	now := time.Now()
	t.Status = TaskStatusFailed
	t.FinishedAt = &now
	return nil
}

// Cancel transitions the task to Cancelled from any non-terminal state.
func (t *DispatchTask) Cancel() error {
	if t.IsFinished() {
		return fmt.Errorf("dispatch: task %s is already in terminal state %q", t.ID, t.Status)
	}
	now := time.Now()
	t.Status = TaskStatusCancelled
	t.FinishedAt = &now
	return nil
}

// NextPendingAction returns the Action with the lowest Sequence that is still Pending.
// Returns nil when all actions have moved past Pending (Running, Completed, Failed, Cancelled).
func (t *DispatchTask) NextPendingAction() *DispatchAction {
	for _, a := range t.sortedActions() {
		if a.Status == ActionStatusPending {
			return a
		}
	}
	return nil
}

// FindCommand looks up a ControlCommand by its ID within the task tree.
// Returns the hosting Action and the Command, or (nil, nil) if not found.
func (t *DispatchTask) FindCommand(commandID string) (*DispatchAction, *ControlCommand) {
	for _, action := range t.Actions {
		for _, cmd := range action.Commands {
			if cmd.ID == commandID {
				return action, cmd
			}
		}
	}
	return nil, nil
}

// IsFinished reports whether the task has reached a terminal state.
func (t *DispatchTask) IsFinished() bool {
	switch t.Status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	}
	return false
}

// Validate returns an error if the task is structurally invalid.
func (t *DispatchTask) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("dispatch: task id is required")
	}
	if strings.TrimSpace(t.TenantID) == "" {
		return errors.New("dispatch: task tenant_id is required")
	}
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("dispatch: task name is required")
	}
	if len(t.Actions) == 0 {
		return errors.New("dispatch: task must have at least one action")
	}
	return nil
}

// sortedActions returns a copy of the Actions slice sorted by Sequence ascending.
// The original slice is not modified.
func (t *DispatchTask) sortedActions() []*DispatchAction {
	sorted := make([]*DispatchAction, len(t.Actions))
	copy(sorted, t.Actions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Sequence < sorted[j].Sequence
	})
	return sorted
}
