package query

import "github.com/mushroomyuan/vpp-backend/resource/domain/model"

// AssetView combines persistent asset metadata with hot runtime state.
type AssetView struct {
	Asset   *model.Asset
	Runtime *model.AssetRuntime
}

// CUView combines persistent CU metadata with connection-plane runtime state.
type CUView struct {
	CU      *model.CU
	Runtime *model.CURuntime
}

// PointView is the persistent metric binding view. Runtime values belong to Telemetry.
type PointView struct {
	Point *model.Point
}
