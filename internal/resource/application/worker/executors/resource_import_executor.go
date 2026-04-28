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

// ResourceImportResult is serialized into import_jobs.result_json when a
// resource import job completes successfully.
type ResourceImportResult struct {
	ResourceIDs []string               `json:"resource_ids"`
	FailedItems []types.BatchItemError `json:"failed_items,omitempty"`
}

// ResourceImportExecutor executes JobTypeResource jobs.
// It streams items in configurable chunks to keep memory usage bounded and
// reports incremental progress back to the job record after each chunk.
type ResourceImportExecutor struct {
	resourceRepo port.ResourceRepository
	jobRepo      port.JobRepository
}

func NewResourceImportExecutor(
	resourceRepo port.ResourceRepository,
	jobRepo port.JobRepository,
) *ResourceImportExecutor {
	return &ResourceImportExecutor{
		resourceRepo: resourceRepo,
		jobRepo:      jobRepo,
	}
}

func (e *ResourceImportExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.ResourceImportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal resource import payload: %w", err)
	}

	allIDs, err := batch.BatchCreateResources(
		ctx,
		e.resourceRepo,
		job.TenantID,
		payload.SiteID,
		payload.Items,
		payload.BatchSize,
		func(succeeded int) error {
			// Report progress after each chunk so the GET endpoint reflects live
			// state. Ignore the error — a failed update must not abort the import.
			job.UpdateProgress(succeeded, 0)
			_ = e.jobRepo.Save(ctx, job)
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	// Write final counts on the in-memory job so processNext can call
	// job.Complete with the right totals. ClaimPending left counters at 0
	// until the last batch; incremental progress is persisted via Save in the callback.
	job.Total = len(payload.Items)
	job.Succeeded = len(allIDs)
	job.FailedCount = 0

	resultJSON, err := json.Marshal(ResourceImportResult{ResourceIDs: allIDs})
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return resultJSON, nil
}
