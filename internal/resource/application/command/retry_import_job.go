package command

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type RetryJob struct {
	JobID string
}

type RetryJobHandler decorator.CommandHandler[RetryJob, struct{}]

type retryJobHandler struct {
	jobRepo port.JobRepository
}

func NewRetryJobHandler(
	jobRepo port.JobRepository,
	metricClient decorator.MetricsClient,
) RetryJobHandler {
	if jobRepo == nil {
		panic("NewRetryJobHandler parameter jobRepo is nil")
	}
	return decorator.ApplyCommandDecorators[RetryJob, struct{}](
		retryJobHandler{jobRepo: jobRepo},
		metricClient,
	)
}

func (h retryJobHandler) Handle(ctx context.Context, cmd RetryJob) (struct{}, error) {
	job, err := h.jobRepo.FindByID(ctx, cmd.JobID)
	if err != nil {
		return struct{}{}, err
	}

	if err := job.ResetForRetry(); err != nil {
		return struct{}{}, err
	}

	if err := h.jobRepo.Save(ctx, job); err != nil {
		return struct{}{}, err
	}

	return struct{}{}, nil
}
