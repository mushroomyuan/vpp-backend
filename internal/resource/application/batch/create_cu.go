package batch

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/resource/application/types"

	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// BatchCreateCUs builds CU domain objects from items and persists them in
// fixed-size chunks. After each chunk, onChunk is called with the running count
// of successfully inserted items; a non-nil return aborts the loop. Pass nil
// for onChunk if no progress callback is needed.
func BatchCreateCUs(
	ctx context.Context,
	cuRepo port.CURepository,
	tenantID string,
	items []types.CUItem,
	batchSize int,
	onChunk func(succeeded int) error,
) ([]string, error) {
	// Validate tenant ID first
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	// Phase 1: validate all items upfront
	var failedItems []types.BatchItemError
	for i, item := range items {
		if err := item.Validate(); err != nil {
			failedItems = append(failedItems, types.BatchItemError{
				Index:  i,
				Name:   item.Name,
				Reason: err.Error(),
			})
			continue
		}
	}

	if len(failedItems) > 0 {
		return nil, fmt.Errorf("%w: %v", types.ErrCUBatchValidation, failedItems)
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

		batch := make([]*model.CU, 0, len(chunk))
		chunkIDs := make([]string, 0, len(chunk))

		for idx, cuItem := range chunk { // 重命名避免混淆
			globalIdx := start + idx
			id := idgen.Must()
			subType := strings.TrimSpace(cuItem.Type)
			var subTypePtr *string
			if subType != "" {
				subTypePtr = &subType
			}

			cu, err := model.NewCU(
				model.CreateCUParams{
					ID:             id,
					TenantID:       tenantID,
					ParentID:       cuItem.ParentID,
					DisplayName:    strings.TrimSpace(cuItem.Name),
					SubType:        subTypePtr,
					Provider:       cuItem.Provider,
					ExternalID:     cuItem.ExternalID,
					Protocol:       cuItem.Protocol,
					Description:    cuItem.Description,
					ProtocolConfig: cuItem.ProtocolConfig,
					Connection:     cuItem.Connection,
					CapabilityTags: cuItem.CapabilityTags,
				},
			)
			if err != nil {
				return nil, fmt.Errorf("build cu at index %d: %w", globalIdx, err)
			}
			if cuItem.Metadata != nil {
				for k, v := range cuItem.Metadata {
					cu.Metadata[k] = v
				}
			}
			batch = append(batch, cu)
			chunkIDs = append(chunkIDs, id)
		}

		if err := cuRepo.BatchCreate(ctx, batch); err != nil {
			return nil, fmt.Errorf("batch insert [%d:%d]: %w", start, end, err)
		}

		succeeded += len(chunk)
		allIDs = append(allIDs, chunkIDs...)

		if onChunk != nil {
			if err := onChunk(succeeded); err != nil {
				// 注意: 此时部分数据已写入数据库，调用方需处理清理逻辑
				return allIDs, err
			}
		}
	}

	return allIDs, nil
}
