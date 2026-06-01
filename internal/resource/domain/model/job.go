package model

import (
	"errors"
	"fmt"
	"time"
)

type JobOperationType string
type JobTargetType string
type JobStatus string

const (
	JobOperationImport JobOperationType = "import"
	JobOperationDelete JobOperationType = "delete"
)

const (
	JobTargetSite  JobTargetType = "site"
	JobTargetAsset JobTargetType = "asset"
	JobTargetCU    JobTargetType = "cu"
	JobTargetPoint JobTargetType = "point"
)

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusSuccess JobStatus = "success"
	JobStatusFailed  JobStatus = "failed"
)

const (
	defaultMaxAttempts      = 3
	retryBackoffAfterFail   = 5 * time.Minute // matches worker poll / DB scheduling behavior
)

// JobKind is the composite key used to look up an executor in the registry.
// It uniquely identifies the combination of operation and target entity.
type JobKind struct {
	Operation JobOperationType
	Target    JobTargetType
}

// Job represents an async batch operation. The Payload field contains
// the job-kind-specific input serialized as JSON. ResultJSON contains the
// outcome (created/deleted IDs, per-item failures, etc.) once the job finishes.
type Job struct {
	ID            string
	TenantID      string
	OperationType JobOperationType
	TargetType    JobTargetType
	Status        JobStatus
	Payload       []byte // JSON-encoded, kind-specific input
	Total         int
	Succeeded     int
	FailedCount   int
	ErrorMsg      string
	ResultJSON    []byte // JSON-encoded outcome, populated on completion
	Attempts      int
	MaxAttempts   int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	StartedAt     *time.Time
	FinishedAt    *time.Time
	NextRetryAt   *time.Time // non-nil after a failed attempt; clears on manual retry
}

// Kind returns the composite registry key for executor dispatch.
func (j *Job) Kind() JobKind {
	return JobKind{Operation: j.OperationType, Target: j.TargetType}
}

func NewJob(id, tenantID string, opType JobOperationType, targetType JobTargetType, payload []byte) (*Job, error) {
	if id == "" {
		return nil, errors.New("job id is required")
	}
	if tenantID == "" {
		return nil, errors.New("tenant id is required")
	}
	if opType == "" {
		return nil, errors.New("job operation type is required")
	}
	if targetType == "" {
		return nil, errors.New("job target type is required")
	}
	if len(payload) == 0 {
		return nil, errors.New("payload is required")
	}
	return &Job{
		ID:            id,
		TenantID:      tenantID,
		OperationType: opType,
		TargetType:    targetType,
		Status:        JobStatusPending,
		Payload:       payload,
		MaxAttempts:   defaultMaxAttempts,
	}, nil
}

// Start transitions the job from pending → running and increments the attempt
// counter. Called by the worker after it claims the job via ClaimPending.
func (j *Job) Start() error {
	if j.Status != JobStatusPending {
		return fmt.Errorf("cannot start job in status %q", j.Status)
	}
	now := time.Now()
	j.Status = JobStatusRunning
	j.Attempts++
	j.StartedAt = &now
	return nil
}

// UpdateProgress records mid-execution counters so observers can track progress
// before the job finishes.
func (j *Job) UpdateProgress(succeeded, failedCount int) {
	j.Succeeded = succeeded
	j.FailedCount = failedCount
}

// Complete transitions running → success and records the final counts.
func (j *Job) Complete(total, succeeded, failedCount int, resultJSON []byte) {
	now := time.Now()
	j.Status = JobStatusSuccess
	j.Total = total
	j.Succeeded = succeeded
	j.FailedCount = failedCount
	j.ResultJSON = resultJSON
	j.FinishedAt = &now
	j.NextRetryAt = nil
}

// Fail transitions running → failed and stores the error message.
func (j *Job) Fail(errMsg string) {
	now := time.Now()
	j.Status = JobStatusFailed
	j.ErrorMsg = errMsg
	j.FinishedAt = &now
	t := now.Add(retryBackoffAfterFail)
	j.NextRetryAt = &t
}

// CanRetry reports whether the job is eligible for another attempt.
func (j *Job) CanRetry() bool {
	return j.Status == JobStatusFailed && j.Attempts < j.MaxAttempts
}

// ResetForRetry transitions a failed job back to pending so the worker will
// pick it up again. NextRetryAt is cleared so the job is immediately eligible.
func (j *Job) ResetForRetry() error {
	if !j.CanRetry() {
		return fmt.Errorf("job cannot be retried (status=%q, attempts=%d/%d)",
			j.Status, j.Attempts, j.MaxAttempts)
	}
	j.Status = JobStatusPending
	j.ErrorMsg = ""
	j.StartedAt = nil
	j.FinishedAt = nil
	j.NextRetryAt = nil
	return nil
}
