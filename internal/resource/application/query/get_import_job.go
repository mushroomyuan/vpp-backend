package query

import (
	"context"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type GetJob struct {
	JobID string
}

type GetJobHandler decorator.QueryHandler[GetJob, *model.Job]

type getJobHandler struct {
	jobRepo port.JobRepository
}

func NewGetJobHandler(
	jobRepo port.JobRepository,
	metricClient decorator.MetricsClient,
) GetJobHandler {
	if jobRepo == nil {
		panic("NewGetJobHandler parameter jobRepo is nil")
	}
	return decorator.ApplyQueryDecorators[GetJob, *model.Job](
		getJobHandler{jobRepo: jobRepo},
		metricClient,
	)
}

func (h getJobHandler) Handle(ctx context.Context, q GetJob) (*model.Job, error) {
	return h.jobRepo.FindByID(ctx, q.JobID)
}
