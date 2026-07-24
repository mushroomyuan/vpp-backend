package batch

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/resource/application/types"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

const defaultBatchSize = 100

// BatchCreateAssets builds Asset domain objects from items and
// persists them in fixed-size chunks. After each chunk, onChunk is called with
// the running count of successfully inserted items; a non-nil return aborts the
// loop. Pass nil for onChunk if no progress callback is needed.
//
// Peak memory stays proportional to batchSize rather than len(items) because
// each chunk slice is released before the next is allocated.
//
// Chunks commit independently. If a later chunk (or onChunk) fails after earlier
// chunks succeeded, already-written IDs are compensated via BatchDelete before
// the error is returned, so RetryJob can safely re-run.
func BatchCreateAssets(
	ctx context.Context,
	assetRepo port.AssetRepository,
	tenantID, siteID string,
	items []types.AssetItem,
	batchSize int,
	onChunk func(succeeded int) error,
) ([]string, error) {
	// Validate tenant ID first.
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	siteID = strings.TrimSpace(siteID)
	var parentID *string
	if siteID != "" {
		parentID = &siteID
	}

	// Validate all items upfront.
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
		return nil, fmt.Errorf("%w: %v", types.ErrAssetBatchValidation, failedItems)
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

		batch := make([]*model.Asset, 0, len(chunk))
		chunkIDs := make([]string, 0, len(chunk))

		for idx, item := range chunk {
			globalIdx := start + idx
			id := idgen.Must()
			subType := normalizeOptionalString(item.SubType)
			dispatchMode := normalizeOptionalString(item.DispatchMode)
			energyType := normalizeOptionalString(item.EnergyType)
			ownerType := normalizeOptionalString(item.OwnerType)
			description := normalizeOptionalString(item.Description)

			ds := item.DispatchStatus
			if strings.TrimSpace(string(ds)) == "" {
				ds = model.DispatchStatusUnknown
			}

			asset, err := model.NewAsset(model.CreateAssetParams{
				ID:              id,
				TenantID:        tenantID,
				ParentID:        parentID,
				DisplayName:     strings.TrimSpace(item.Name),
				DispatchStatus:  ds,
				RatedCapacityKW: item.RatedCapacityKW,
				DispatchMode:    dispatchMode,
				EnergyType:      energyType,
				OwnerType:       ownerType,
				SubType:         subType,
				Description:     description,
				MarketEnabled:   item.MarketEnabled,
			})
			if err != nil {
				return nil, compensateCreated(ctx, tenantID, allIDs, assetRepo.BatchDelete,
					fmt.Errorf("build asset at index %d: %w", globalIdx, err))
			}
			if item.Metadata != nil {
				for k, v := range item.Metadata {
					asset.Metadata[k] = v
				}
			}
			batch = append(batch, asset)
			chunkIDs = append(chunkIDs, id)
		}

		if err := assetRepo.BatchCreate(ctx, batch); err != nil {
			return nil, compensateCreated(ctx, tenantID, allIDs, assetRepo.BatchDelete,
				fmt.Errorf("batch insert [%d:%d]: %w", start, end, err))
		}

		succeeded += len(chunk)
		allIDs = append(allIDs, chunkIDs...)

		if onChunk != nil {
			if err := onChunk(succeeded); err != nil {
				return nil, compensateCreated(ctx, tenantID, allIDs, assetRepo.BatchDelete, err)
			}
		}
	}

	return allIDs, nil
}

func normalizeOptionalString(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
