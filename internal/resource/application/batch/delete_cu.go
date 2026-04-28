package batch

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// BatchDeleteCUs soft-deletes CU ids in fixed-size chunks. After each
// successful chunk, onChunk is called with the cumulative count of ids
// processed; a non-nil return aborts the loop. Pass nil for onChunk if no
// progress callback is needed.
func BatchDeleteCUs(
	ctx context.Context,
	cuRepo port.CURepository,
	tenantID string,
	ids []string,
	batchSize int,
	onChunk func(succeeded int) error,
) error {
	if len(ids) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	succeeded := 0
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]

		if err := cuRepo.BatchDelete(ctx, tenantID, chunk); err != nil {
			return fmt.Errorf("batch delete cus [%d:%d]: %w", start, end, err)
		}
		succeeded += len(chunk)
		if onChunk != nil {
			if err := onChunk(succeeded); err != nil {
				return err
			}
		}
	}
	return nil
}
