package domain

import "errors"

var (
	ErrTaskNotFound = errors.New("dispatch: task not found")
	// ErrIdempotencyConflict means this tenant already has a task for the key.
	ErrIdempotencyConflict = errors.New("dispatch: task idempotency key already exists")
	ErrCommandNotFound     = errors.New("dispatch: command not found in task")
	ErrTaskNotRunning      = errors.New("dispatch: task is not in running state")
	ErrTaskAlreadyDone     = errors.New("dispatch: task is already in a terminal state")
)
