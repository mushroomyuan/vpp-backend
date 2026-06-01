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

// PointView combines point definition with latest cached value.
type PointView struct {
	Point   *model.Point
	Runtime *model.PointRuntime
}
