package query

import (
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// defaultStaleAge is the deployment default for snapshot staleness checks.
// Both GetSnapshot and GetFleetSnapshot use this when no StaleAge is provided.
const defaultStaleAge = 5 * time.Minute

// SnapshotView is the application-layer read model for a CU's real-time state.
// It wraps the domain Snapshot and adds the derived Stale flag, which depends
// on "what time is it now" — a runtime concern that intentionally stays outside
// the domain model.
type SnapshotView struct {
	TenantID  string
	CUCode    string
	Metrics   map[string]float64
	UpdatedAt time.Time
	Stale     bool
}

// snapshotToView converts a domain Snapshot to a SnapshotView.
// Pass staleAge == 0 to skip the staleness check.
func snapshotToView(s *model.Snapshot, staleAge time.Duration) *SnapshotView {
	return &SnapshotView{
		TenantID:  s.TenantID,
		CUCode:    s.CUCode,
		Metrics:   s.Metrics,
		UpdatedAt: s.UpdatedAt,
		Stale:     staleAge > 0 && s.IsStale(staleAge),
	}
}
