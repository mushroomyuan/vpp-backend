package service

import (
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
)

// Validator enforces structural and business-rule constraints on domain objects
// before they are persisted or executed.
//
// It is stateless and side-effect free; construct once and reuse.
type Validator struct{}

// NewValidator constructs a Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateTask checks the Task and all its nested Actions and Commands.
// It also enforces that Action Sequence numbers are unique within a task.
func (v *Validator) ValidateTask(task *model.DispatchTask) error {
	if task == nil {
		return errors.New("dispatch: task must not be nil")
	}
	if err := task.Validate(); err != nil {
		return err
	}
	seqSet := make(map[int]bool, len(task.Actions))
	for i, action := range task.Actions {
		if action == nil {
			return fmt.Errorf("dispatch: action at index %d is nil", i)
		}
		if seqSet[action.Sequence] {
			return fmt.Errorf("dispatch: duplicate action sequence %d", action.Sequence)
		}
		seqSet[action.Sequence] = true
		if err := v.ValidateAction(action); err != nil {
			return fmt.Errorf("dispatch: invalid action %q (seq=%d): %w", action.Name, action.Sequence, err)
		}
	}
	return nil
}

// ValidateAction checks the Action and all its Commands.
func (v *Validator) ValidateAction(action *model.DispatchAction) error {
	if action == nil {
		return errors.New("dispatch: action must not be nil")
	}
	if action.Name == "" {
		return errors.New("dispatch: action name is required")
	}
	if len(action.Commands) == 0 {
		return errors.New("dispatch: action must have at least one command")
	}
	for i, cmd := range action.Commands {
		if cmd == nil {
			return fmt.Errorf("dispatch: command at index %d is nil", i)
		}
		if err := v.ValidateCommand(cmd); err != nil {
			return fmt.Errorf("dispatch: invalid command at index %d: %w", i, err)
		}
	}
	return nil
}

// ValidateCommand checks a single ControlCommand.
func (v *Validator) ValidateCommand(cmd *model.ControlCommand) error {
	if cmd == nil {
		return errors.New("dispatch: command must not be nil")
	}
	return cmd.Validate()
}
