package batch

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/resource/application/types"

	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// BatchCreatePoints builds Point domain objects from items and persists
// them in fixed-size chunks. After each chunk, onChunk is called with the
// running count of successfully inserted items; a non-nil return aborts the
// loop. Pass nil for onChunk if no progress callback is needed.
func BatchCreatePoints(
	ctx context.Context,
	pointRepo port.PointRepository,
	resourceID, cuID string,
	items []types.PointItem,
	batchSize int,
	onChunk func(succeeded int) error,
) ([]string, error) {
	// Phase 1: validate all items upfront so the caller receives every error at
	// once instead of discovering them one batch at a time.
	var failedItems []types.BatchItemError
	for i, item := range items {
		if err := item.Validate(); err != nil {
			failedItems = append(failedItems, types.BatchItemError{
				Index:  i,
				Name:   item.PointKey,
				Reason: err.Error(),
			})
		}
	}
	if len(failedItems) > 0 {
		return nil, fmt.Errorf("%w: %v", types.ErrPointBatchValidation, failedItems)
	}

	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}

	allIDs := make([]string, 0, len(items))
	succeeded := 0

	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]

		batch := make([]*model.Point, 0, len(chunk))
		chunkIDs := make([]string, 0, len(chunk))

		for _, item := range chunk {
			id := idgen.Must()
			point, err := model.NewPoint(
				id,
				resourceID,
				cuID,
				item.PointKey,
				item.ExternalAddress,
				item.DataType,
				item.ExtConfig,
				item.Description,
				item.ControlFlag,
				item.IsVirtual,
				item.SafetyThresholds,
				item.CacheKeyAlias,
			)
			if err != nil {
				return nil, fmt.Errorf("build point at index %d: %w", start+len(chunkIDs), err)
			}
			batch = append(batch, point)
			chunkIDs = append(chunkIDs, id)
		}

		if err := pointRepo.BatchCreate(ctx, batch); err != nil {
			return nil, fmt.Errorf("batch insert [%d:%d]: %w", start, end, err)
		}

		succeeded += len(chunk)
		allIDs = append(allIDs, chunkIDs...)

		if onChunk != nil {
			if err := onChunk(succeeded); err != nil {
				return allIDs, err
			}
		}
	}

	return allIDs, nil
}
