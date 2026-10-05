package query

import "github.com/mushroomyuan/vpp-backend/resource/domain/model"

// AssetView is the persistent asset catalog view.
// Live measurements belong to Telemetry snapshots.
type AssetView struct {
	Asset *model.Asset
}

// CUView is the persistent control-unit catalog view.
// Connection diagnostics and live measurements are outside Resource.
type CUView struct {
	CU *model.CU
}

// PointView is the persistent metric binding view. Runtime values belong to Telemetry.
type PointView struct {
	Point *model.Point
}
