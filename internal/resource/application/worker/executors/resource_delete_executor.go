package executors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/application/batch"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// ResourceDeleteResult is stored in import_jobs.result_json on successful batch delete.
type ResourceDeleteResult struct {
	DeletedCount int `json:"deleted_count"`
}

// ResourceDeleteExecutor runs JobOperationDelete + JobTargetResource jobs.
type ResourceDeleteExecutor struct {
	resourceRepo port.ResourceRepository
	jobRepo      port.JobRepository
}

func NewResourceDeleteExecutor(
	resourceRepo port.ResourceRepository,
	jobRepo port.JobRepository,
) *ResourceDeleteExecutor {
	return &ResourceDeleteExecutor{
		resourceRepo: resourceRepo,
		jobRepo:      jobRepo,
	}
}

func (e *ResourceDeleteExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.ResourceDeletePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal resource delete payload: %w", err)
	}

	if err := batch.BatchDeleteResources(
		ctx,
		e.resourceRepo,
		job.TenantID,
		payload.IDs,
		payload.BatchSize,
		func(succeeded int) error {
			job.UpdateProgress(succeeded, 0)
			_ = e.jobRepo.Save(ctx, job)
			return nil
		},
	); err != nil {
		return nil, err
	}

	job.Total = len(payload.IDs)
	job.Succeeded = len(payload.IDs)
	job.FailedCount = 0

	resultJSON, err := json.Marshal(ResourceDeleteResult{DeletedCount: len(payload.IDs)})
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return resultJSON, nil
}
