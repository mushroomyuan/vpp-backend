package port

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

// JobRepository manages the persistence of async import jobs.
type JobRepository interface {
	// Create persists a new job in pending status.
	Create(ctx context.Context, job *model.Job) error

	// FindByID returns the job or domain.ErrJobNotFound.
	FindByID(ctx context.Context, id string) (*model.Job, error)

	// ClaimPending atomically finds the oldest pending job whose attempt count
	// is below MaxAttempts, transitions it to running, and returns it.
	// Returns nil, nil when no pending jobs are available.
	// Implemented via SELECT FOR UPDATE SKIP LOCKED to be safe under multi-node
	// deployment without requiring an external lock service.
	ClaimPending(ctx context.Context) (*model.Job, error)

	// Save persists the current state of a job. Callers mutate the aggregate
	// via its own methods (UpdateProgress, Complete, Fail, ResetForRetry) and
	// then call Save once — keeping all state-transition logic inside the domain.
	Save(ctx context.Context, job *model.Job) error
}
