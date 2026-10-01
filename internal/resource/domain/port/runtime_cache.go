package port

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/resource/domain/model"
)

// --- Asset Runtime ---

type AssetRuntimeReader interface {
	GetAssetRuntime(ctx context.Context, tenantID, assetID string) (*model.AssetRuntime, error)
	// 批量查询是 VPP 调度的核心路径，必须一等公民
	ListAssetRuntimes(ctx context.Context, tenantID string, assetIDs []string) ([]*model.AssetRuntime, error)
}

type AssetRuntimeWriter interface {
	SetAssetRuntime(ctx context.Context, r *model.AssetRuntime) error
	// 部分更新：设备上报电流值，不应覆盖 SOC 等其他字段
	PatchAssetRuntime(ctx context.Context, tenantID, assetID string, patch AssetRuntimePatch) error
	DeleteAssetRuntime(ctx context.Context, tenantID, assetID string) error
}

type AssetRuntimeCache interface {
	AssetRuntimeReader
	AssetRuntimeWriter
}

// --- CU Runtime ---

type CURuntimeReader interface {
	GetCURuntime(ctx context.Context, tenantID, cuID string) (*model.CURuntime, error)
	ListCURuntimes(ctx context.Context, tenantID string, cuIDs []string) ([]*model.CURuntime, error)
}

type CURuntimeWriter interface {
	SetCURuntime(ctx context.Context, r *model.CURuntime) error
	PatchCURuntime(ctx context.Context, tenantID, cuID string, patch CURuntimePatch) error
	DeleteCURuntime(ctx context.Context, tenantID, cuID string) error
}

type CURuntimeCache interface {
	CURuntimeReader
	CURuntimeWriter
}

type AssetRuntimePatch struct {
	Online                *bool
	CurrentPowerKW        *float64
	AvailablePowerKW      *float64
	SOC                   *float64
	Dispatchable          *bool
	NotDispatchableReason *string
	MaxChargePowerKW      *float64
	MaxDischargePowerKW   *float64
}

type CURuntimePatch struct {
	ConnStatus *string
	LastSeenAt *time.Time
	LatencyMS  *int64
	LastError  *string
}
