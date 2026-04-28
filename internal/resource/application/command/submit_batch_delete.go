package command

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/mushroomyuan/vpp-backend/platform/decorator"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// ── Command + result ──────────────────────────────────────────────────────────

// SubmitBatchDelete enqueues an async batch soft-delete job for one target type.
// Exactly one of Resource / CU / Point must be set (same mutual-exclusion pattern as SubmitBatchImport).
type SubmitBatchDelete struct {
	BatchSize int // 0 = executor default

	Resource *types.ResourceDeleteSpec
	CU       *types.CUDeleteSpec
	Point    *types.PointDeleteSpec
}

type SubmitBatchDeleteResult struct {
	JobID       string
	FailedItems []types.BatchItemError // set (with ErrBatchValidation) on validation failure
}

// ── Handler ───────────────────────────────────────────────────────────────────

type SubmitBatchDeleteHandler decorator.CommandHandler[SubmitBatchDelete, *SubmitBatchDeleteResult]

type submitBatchDeleteHandler struct {
	jobRepo port.JobRepository
}

func NewSubmitBatchDeleteHandler(
	jobRepo port.JobRepository,
	metricClient decorator.MetricsClient,
) SubmitBatchDeleteHandler {
	if jobRepo == nil {
		panic("NewSubmitBatchDeleteHandler: jobRepo is nil")
	}
	return decorator.ApplyCommandDecorators[SubmitBatchDelete, *SubmitBatchDeleteResult](
		submitBatchDeleteHandler{jobRepo: jobRepo},
		metricClient,
	)
}

func (h submitBatchDeleteHandler) Handle(
	ctx context.Context,
	cmd SubmitBatchDelete,
) (*SubmitBatchDeleteResult, error) {
	ctx, span := telemetry.Start(ctx, "submit_batch_delete")
	defer span.End()

	targetType, tenantID, payload, failedItems, err := buildDeleteJobPayload(cmd)
	if err != nil {
		return nil, err
	}
	if len(failedItems) > 0 {
		return &SubmitBatchDeleteResult{FailedItems: failedItems}, types.ErrBatchDeleteValidation
	}

	job, err := model.NewJob(idgen.Must(), tenantID, model.JobOperationDelete, targetType, payload)
	if err != nil {
		return nil, err
	}
	if err := h.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}
	return &SubmitBatchDeleteResult{JobID: job.ID}, nil
}

func buildDeleteJobPayload(cmd SubmitBatchDelete) (
	targetType model.JobTargetType,
	tenantID string,
	payload []byte,
	failedItems []types.BatchItemError,
	err error,
) {
	switch {
	case cmd.Resource != nil:
		s := cmd.Resource
		if s.TenantID == "" {
			err = errors.New("SubmitBatchDelete: Resource.TenantID is required")
			return
		}
		failedItems = validateDeleteIDs(s.IDs)
		if len(failedItems) > 0 {
			return
		}
		targetType = model.JobTargetResource
		tenantID = s.TenantID
		payload, err = json.Marshal(types.ResourceDeletePayload{
			BatchSize: cmd.BatchSize,
			IDs:       s.IDs,
		})

	case cmd.CU != nil:
		s := cmd.CU
		if s.TenantID == "" {
			err = errors.New("SubmitBatchDelete: CU.TenantID is required")
			return
		}
		failedItems = validateDeleteIDs(s.IDs)
		if len(failedItems) > 0 {
			return
		}
		targetType = model.JobTargetCU
		tenantID = s.TenantID
		payload, err = json.Marshal(types.CUDeletePayload{
			BatchSize: cmd.BatchSize,
			IDs:       s.IDs,
		})

	case cmd.Point != nil:
		s := cmd.Point
		if s.TenantID == "" {
			err = errors.New("SubmitBatchDelete: Point.TenantID is required")
			return
		}
		failedItems = validateDeleteIDs(s.IDs)
		if len(failedItems) > 0 {
			return
		}
		targetType = model.JobTargetPoint
		tenantID = s.TenantID
		payload, err = json.Marshal(types.PointDeletePayload{
			BatchSize: cmd.BatchSize,
			IDs:       s.IDs,
		})

	default:
		err = errors.New("SubmitBatchDelete: exactly one of Resource, CU, or Point must be set")
	}
	return
}

func validateDeleteIDs(ids []string) []types.BatchItemError {
	if len(ids) == 0 {
		return []types.BatchItemError{{Index: 0, Name: "ids", Reason: "at least one id is required"}}
	}
	var out []types.BatchItemError
	for i, id := range ids {
		if id == "" {
			out = append(out, types.BatchItemError{Index: i, Name: id, Reason: "id must not be empty"})
		}
	}
	return out
}
