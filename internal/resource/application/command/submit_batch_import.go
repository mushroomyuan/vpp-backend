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

// SubmitBatchImport submits an async batch-import job for one resource type.
// gRPC/proto oneof is mutually exclusive; only one of Resource / CU / Point should be set.
// BatchSize is set inside the embedded Payload when constructing the Spec.
type SubmitBatchImport struct {
	Asset *types.AssetImportSpec
	CU    *types.CUImportSpec
	Point *types.PointImportSpec
}

type SubmitBatchImportResult struct {
	JobID       string
	FailedItems []types.BatchItemError // set (with ErrBatchValidation) on validation failure
}

// ── Handler ───────────────────────────────────────────────────────────────────

type SubmitBatchImportHandler decorator.CommandHandler[SubmitBatchImport, *SubmitBatchImportResult]

type submitBatchImportHandler struct {
	jobRepo port.JobRepository
}

func NewSubmitBatchImportHandler(
	jobRepo port.JobRepository,
	metricClient decorator.MetricsClient,
) SubmitBatchImportHandler {
	if jobRepo == nil {
		panic("NewSubmitBatchImportHandler: jobRepo is nil")
	}
	return decorator.ApplyCommandDecorators[SubmitBatchImport, *SubmitBatchImportResult](
		submitBatchImportHandler{jobRepo: jobRepo},
		metricClient,
	)
}

func (h submitBatchImportHandler) Handle(
	ctx context.Context,
	cmd SubmitBatchImport,
) (*SubmitBatchImportResult, error) {
	ctx, span := telemetry.Start(ctx, "submit_batch_import")
	defer span.End()

	targetType, tenantID, payload, failedItems, err := buildJobPayload(cmd)
	if err != nil {
		return nil, err
	}
	if len(failedItems) > 0 {
		return &SubmitBatchImportResult{FailedItems: failedItems}, types.ErrBatchImportValidation
	}

	job, err := model.NewJob(idgen.Must(), tenantID, model.JobOperationImport, targetType, payload)
	if err != nil {
		return nil, err
	}
	if err := h.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}
	return &SubmitBatchImportResult{JobID: job.ID}, nil
}

// buildJobPayload: target dispatch, per-item validation, and JSON payload in one place.
// Each Spec embeds its Payload directly, so marshaling requires no field copying.
func buildJobPayload(cmd SubmitBatchImport) (
	targetType model.JobTargetType,
	tenantID string,
	payload []byte,
	failedItems []types.BatchItemError,
	err error,
) {
	switch {
	case cmd.Asset != nil:
		s := cmd.Asset
		failedItems = validateAssetItems(s.Items)
		if len(failedItems) > 0 {
			return
		}
		targetType = model.JobTargetAsset
		tenantID = s.TenantID
		payload, err = json.Marshal(s.AssetImportPayload)

	case cmd.CU != nil:
		s := cmd.CU
		failedItems = validateCUItems(s.Items)
		if len(failedItems) > 0 {
			return
		}
		targetType = model.JobTargetCU
		tenantID = s.TenantID
		payload, err = json.Marshal(s.CUImportPayload)

	case cmd.Point != nil:
		s := cmd.Point
		failedItems = validatePointItems(s.Items)
		if len(failedItems) > 0 {
			return
		}
		targetType = model.JobTargetPoint
		tenantID = s.TenantID
		payload, err = json.Marshal(s.PointImportPayload)

	default:
		err = errors.New("SubmitBatchImport: exactly one of Resource, CU, or Point must be set")
	}
	return
}

func validateAssetItems(items []types.AssetItem) []types.BatchItemError {
	var out []types.BatchItemError
	for i, item := range items {
		if err := item.Validate(); err != nil {
			out = append(out, types.BatchItemError{Index: i, Name: item.Name, Reason: err.Error()})
		}
	}
	return out
}

func validateCUItems(items []types.CUItem) []types.BatchItemError {
	var out []types.BatchItemError
	for i, item := range items {
		if err := item.Validate(); err != nil {
			out = append(out, types.BatchItemError{Index: i, Name: item.Name, Reason: err.Error()})
		}
	}
	return out
}

func validatePointItems(items []types.PointItem) []types.BatchItemError {
	var out []types.BatchItemError
	for i, item := range items {
		if err := item.Validate(); err != nil {
			out = append(out, types.BatchItemError{Index: i, Name: item.PointKey, Reason: err.Error()})
		}
	}
	return out
}
