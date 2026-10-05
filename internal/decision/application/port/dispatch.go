package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
	"github.com/mushroomyuan/vpp-backend/decision/domain/model"
)

// Command is one concrete control instruction. MetricID is the canonical
// metric, written into Dispatch's PointKey string by the outbound adapter.
type Command struct {
	CUCode   string
	MetricID contracts.MetricID
	Value    model.CommandValue

	// TimeoutSeconds and MaxRetries use 0 to mean the dispatch service default.
	TimeoutSeconds int32
	MaxRetries     int32
}

// Task is one automatic dispatch submission. Commands run in parallel.
type Task struct {
	TenantID string
	Name     string
	// IdempotencyKey is the Dispatch dedupe key. Plan execution uses
	// plan-step:{stepID} and keeps that value across timeout retries.
	IdempotencyKey string
	Commands       []Command
}

// SubmitResult is the dispatch acknowledgement.
type SubmitResult struct {
	TaskID string
	Status string
}

// DispatchPort submits concrete commands. It does not understand policies or scopes.
type DispatchPort interface {
	SubmitTask(ctx context.Context, task Task) (SubmitResult, error)
}
