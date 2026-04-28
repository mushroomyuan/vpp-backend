package worker

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
	"github.com/sirupsen/logrus"
)

const defaultPollInterval = 5 * time.Second

// ImportWorker is a background goroutine that polls import_jobs for pending
// work, dispatches each job to the appropriate Executor, and updates the final
// status. A single goroutine per process is intentional: for the VPP MVP
// scale this is sufficient, and it avoids the complexity of a concurrent
// worker pool while still being safe under multi-node deployment (the DB-level
// SELECT FOR UPDATE SKIP LOCKED prevents two pods from running the same job).
type ImportWorker struct {
	jobRepo   port.JobRepository
	executors ExecutorRegistry
	interval  time.Duration
}

type ImportWorkerConfig struct {
	// PollInterval controls how often the worker checks for new pending jobs.
	// Defaults to 5 s when zero.
	PollInterval time.Duration
}

func NewImportWorker(
	jobRepo port.JobRepository,
	executors ExecutorRegistry,
	cfg ImportWorkerConfig,
) *ImportWorker {
	interval := cfg.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	return &ImportWorker{
		jobRepo:   jobRepo,
		executors: executors,
		interval:  interval,
	}
}

// Start begins the poll loop. It blocks until ctx is cancelled, making it
// suitable for launching as a goroutine: go worker.Start(ctx).
func (w *ImportWorker) Start(ctx context.Context) {
	logrus.Info("import worker started")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logrus.Info("import worker stopped")
			return
		case <-ticker.C:
			w.processNext(ctx)
		}
	}
}

// processNext claims and executes at most one pending job per tick.
// If there is no pending work it returns immediately without logging.
func (w *ImportWorker) processNext(ctx context.Context) {
	job, err := w.jobRepo.ClaimPending(ctx)
	if err != nil {
		logrus.WithError(err).Error("import worker: claim pending job failed")
		return
	}
	if job == nil {
		return // no pending work
	}

	fields := logrus.Fields{
		"job_id":    job.ID,
		"operation": job.OperationType,
		"target":    job.TargetType,
		"attempt":   job.Attempts,
	}
	logrus.WithFields(fields).Info("import worker: executing job")

	executor, err := w.executors.Get(job.Kind())
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("import worker: no executor for job type")
		job.Fail(err.Error())
		_ = w.jobRepo.Save(ctx, job)
		return
	}

	resultJSON, execErr := executor.Execute(ctx, job)
	if execErr != nil {
		logrus.WithFields(fields).WithError(execErr).Error("import worker: job execution failed")
		job.Fail(execErr.Error())
		_ = w.jobRepo.Save(ctx, job)
		return
	}

	job.Complete(job.Total, job.Succeeded, job.FailedCount, resultJSON)
	if err := w.jobRepo.Save(ctx, job); err != nil {
		logrus.WithFields(fields).WithError(err).Error("import worker: failed to mark job complete")
		return
	}

	logrus.WithFields(fields).Info("import worker: job completed")
}
