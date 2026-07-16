package executors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/resource/application/batch"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

type CUImportResult struct {
	CUIDs       []string               `json:"cu_ids"`
	FailedItems []types.BatchItemError `json:"failed_items,omitempty"`
}

// CUImportExecutor executes JobTypeCU jobs.
type CUImportExecutor struct {
	cuRepo    port.CURepository
	jobRepo   port.JobRepository
	publisher port.ResourceEventPublisher
}

func NewCUImportExecutor(
	cuRepo port.CURepository,
	jobRepo port.JobRepository,
	publisher port.ResourceEventPublisher,
) *CUImportExecutor {
	return &CUImportExecutor{
		cuRepo:    cuRepo,
		jobRepo:   jobRepo,
		publisher: publisher,
	}
}

func (e *CUImportExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.CUImportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal cu import payload: %w", err)
	}

	allIDs, err := batch.BatchCreateCUs(
		ctx,
		e.cuRepo,
		job.TenantID,
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

	resultJSON, err := json.Marshal(CUImportResult{CUIDs: allIDs})
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
			logging.Warnf(ctx, logrus.Fields{
				"tenant_id":   job.TenantID,
				"resource_id": job.ID,
				"error":       pubErr.Error(),
			}, "failed to publish CU import completed event")
		}
	}

	return resultJSON, nil
}
