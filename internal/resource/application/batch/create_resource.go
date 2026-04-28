package batch

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

const defaultBatchSize = 100

// BatchCreateResources builds Resource domain objects from items and
// persists them in fixed-size chunks. After each chunk, onChunk is called with
// the running count of successfully inserted items; a non-nil return aborts the
// loop. Pass nil for onChunk if no progress callback is needed.
//
// Peak memory stays proportional to batchSize rather than len(items) because
// each chunk slice is released before the next is allocated.
func BatchCreateResources(
	ctx context.Context,
	resourceRepo port.ResourceRepository,
	tenantID, siteID string,
	items []types.ResourceItem,
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
				Name:   item.Name,
				Reason: err.Error(),
			})
		}
	}
	if len(failedItems) > 0 {
		return nil, fmt.Errorf("%w: %v", types.ErrResourceBatchValidation, failedItems)
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

		batch := make([]*model.Resource, 0, len(chunk))
		chunkIDs := make([]string, 0, len(chunk))

		for _, item := range chunk {
			id := idgen.Must()
			resource, err := model.NewResource(
				id,
				tenantID,
				siteID,
				item.Name,
				item.Type,
				item.Capacity,
				item.Manufacturer,
				item.Model,
				item.Metadata,
			)
			if err != nil {
				return nil, fmt.Errorf("build resource at index %d: %w", start+len(chunkIDs), err)
			}
			batch = append(batch, resource)
			chunkIDs = append(chunkIDs, id)
		}

		if err := resourceRepo.BatchCreate(ctx, batch); err != nil {
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
