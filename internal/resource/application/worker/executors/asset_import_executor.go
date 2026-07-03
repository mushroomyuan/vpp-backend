package executors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/resource/application/batch"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"

	platEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
)

// AssetImportResult is serialized into import_jobs.result_json when an
// asset import job completes successfully.
type AssetImportResult struct {
	AssetIDs    []string               `json:"asset_ids"`
	FailedItems []types.BatchItemError `json:"failed_items,omitempty"`
}

// AssetImportExecutor executes JobOperationImport + JobTargetAsset jobs.
// It streams items in configurable chunks to keep memory usage bounded and
// reports incremental progress back to the job record after each chunk.
type AssetImportExecutor struct {
	assetRepo port.AssetRepository
	jobRepo   port.JobRepository
	publisher port.ResourceEventPublisher
}

func NewAssetImportExecutor(
	assetRepo port.AssetRepository,
	jobRepo port.JobRepository,
	publisher port.ResourceEventPublisher,
) *AssetImportExecutor {
	return &AssetImportExecutor{
		assetRepo: assetRepo,
		jobRepo:   jobRepo,
		publisher: publisher,
	}
}

func (e *AssetImportExecutor) Execute(ctx context.Context, job *model.Job) ([]byte, error) {
	var payload types.AssetImportPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal asset import payload: %w", err)
	}

	allIDs, err := batch.BatchCreateAssets(
		ctx,
		e.assetRepo,
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

	resultJSON, err := json.Marshal(AssetImportResult{
		AssetIDs: allIDs,
	})
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
			logrus.WithError(pubErr).Warn("failed to publish asset import completed event")
		}
	}

	return resultJSON, nil
}
