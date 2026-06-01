package executors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/application/batch"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// PointDeleteResult is stored in import_jobs.result_json on successful batch delete.
type PointDeleteResult struct {
	DeletedCount int `json:"deleted_count"`
}

// PointDeleteExecutor runs JobOperationDelete + JobTargetPoint jobs.
type PointDeleteExecutor struct {
	pointRepo port.PointRepository
	jobRepo   port.JobRepository
}

func NewPointDeleteExecutor(
	pointRepo port.PointRepository,
	jobRepo port.JobRepository,
) *PointDeleteExecutor {
	return &PointDeleteExecutor{
		pointRepo: pointRepo,
		jobRepo:   jobRepo,
	}
}

func (e *PointDeleteExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.PointDeletePayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal point delete payload: %w", err)
	}

	succeeded := 0
	failed := 0
	if err := batch.BatchDeletePoints(
		ctx,
		e.pointRepo,
		job.TenantID,
		payload.IDs,
		payload.BatchSize,
		func(succeeded int) error {
			job.UpdateProgress(succeeded, 0)
			_ = e.jobRepo.Save(ctx, job)
			return nil
		},
	); err != nil {
		var partialErr *batch.BatchDeletePointsPartialError
		if errors.As(err, &partialErr) {
			succeeded = partialErr.Succeeded
			failed = len(partialErr.FailedIDs)
			job.Total = len(payload.IDs)
			job.Succeeded = succeeded
			job.FailedCount = failed
		}
		return nil, err
	}

	job.Total = len(payload.IDs)
	job.Succeeded = len(payload.IDs)
	job.FailedCount = 0

	resultJSON, err := json.Marshal(PointDeleteResult{DeletedCount: len(payload.IDs)})
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return resultJSON, nil
}
