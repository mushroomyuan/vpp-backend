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

// CUDeleteResult is stored in import_jobs.result_json on successful batch delete.
type CUDeleteResult struct {
	DeletedCount int `json:"deleted_count"`
}

// CUDeleteExecutor runs JobOperationDelete + JobTargetCU jobs.
type CUDeleteExecutor struct {
	cuRepo  port.CURepository
	jobRepo port.JobRepository
}

func NewCUDeleteExecutor(
	cuRepo port.CURepository,
	jobRepo port.JobRepository,
) *CUDeleteExecutor {
	return &CUDeleteExecutor{
		cuRepo:  cuRepo,
		jobRepo: jobRepo,
	}
}

func (e *CUDeleteExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.CUDeletePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal cu delete payload: %w", err)
	}

	if err := batch.BatchDeleteCUs(
		ctx,
		e.cuRepo,
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

	resultJSON, err := json.Marshal(CUDeleteResult{DeletedCount: len(payload.IDs)})
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return resultJSON, nil
}
