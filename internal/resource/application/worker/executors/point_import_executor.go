package executors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/resource/application/batch"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type PointImportResult struct {
	PointIDs    []string               `json:"point_ids"`
	FailedItems []types.BatchItemError `json:"failed_items,omitempty"`
}

// PointImportExecutor executes JobTypePoint jobs.
type PointImportExecutor struct {
	pointRepo port.PointRepository
	jobRepo   port.JobRepository
	publisher port.ResourceEventPublisher
}

func NewPointImportExecutor(
	pointRepo port.PointRepository,
	jobRepo port.JobRepository,
	publisher port.ResourceEventPublisher,
) *PointImportExecutor {
	return &PointImportExecutor{
		pointRepo: pointRepo,
		jobRepo:   jobRepo,
		publisher: publisher,
	}
}

func (e *PointImportExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.PointImportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal point import payload: %w", err)
	}

	allIDs, err := batch.BatchCreatePoints(
		ctx,
		job.TenantID,
		e.pointRepo,
		payload.AssetID,
		payload.CUID,
		payload.Items,
		payload.BatchSize,
		func(succeeded int) error {
			job.UpdateProgress(succeeded, 0)
			_ = e.jobRepo.Save(ctx, job)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	job.Total = len(payload.Items)
	job.Succeeded = len(allIDs)
	job.FailedCount = 0

	resultJSON, err := json.Marshal(PointImportResult{PointIDs: allIDs})
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	if e.publisher != nil {
		if pubErr := e.publisher.Publish(ctx, port.ResourceEvent{
			EventType:  platEvent.TypeImportCompleted,
			TenantID:   job.TenantID,
			ResourceID: job.ID,
			Payload: platEvent.ImportCompletedPayload{
				JobID:      job.ID,
				TenantID:   job.TenantID,
				Operation:  string(job.OperationType),
				TargetType: string(job.TargetType),
				Total:      job.Total,
				Succeeded:  job.Succeeded,
				Failed:     job.FailedCount,
			},
		}); pubErr != nil {
			logrus.WithError(pubErr).Warn("failed to publish point import completed event")
		}
	}

	return resultJSON, nil
}
