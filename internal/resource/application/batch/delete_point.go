package batch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// BatchDeletePointsPartialError indicates a non-atomic delete where some chunks
// succeeded and one or more chunks failed.
type BatchDeletePointsPartialError struct {
	Succeeded int
	FailedIDs []string
	Cause     error
}

func (e *BatchDeletePointsPartialError) Error() string {
	return fmt.Sprintf(
		"partial batch delete: succeeded=%d failed=%d: %v",
		e.Succeeded,
		len(e.FailedIDs),
		e.Cause,
	)
}

func (e *BatchDeletePointsPartialError) Unwrap() error { return e.Cause }

// BatchDeletePoints soft-deletes point ids in fixed-size chunks. Tenant
// scoping matches List/SoftDelete (via assets join). After each successful
// chunk, onChunk is called with the cumulative count of ids processed; a
// non-nil return aborts the loop.
func BatchDeletePoints(
	ctx context.Context,
	pointRepo port.PointRepository,
	tenantID string,
	ids []string,
	batchSize int,
	onChunk func(succeeded int) error,
) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if len(ids) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	succeeded := 0
	failedIDs := make([]string, 0)
	errs := make([]error, 0)
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		if err := pointRepo.BatchDelete(ctx, tenantID, chunk); err != nil {
			failedIDs = append(failedIDs, chunk...)
			errs = append(errs, fmt.Errorf("batch delete points [%d:%d]: %w", start, end, err))
			continue
		}
		succeeded += len(chunk)
		if onChunk != nil {
			if err := onChunk(succeeded); err != nil {
				return err
			}
		}
	}
	if len(errs) > 0 {
		return &BatchDeletePointsPartialError{
			Succeeded: succeeded,
			FailedIDs: failedIDs,
			Cause:     errors.Join(errs...),
		}
	}
	return nil
}
