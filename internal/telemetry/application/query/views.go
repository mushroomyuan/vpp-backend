package query

import (
	"sort"
	"time"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
)

// defaultStaleAge is the deployment default for snapshot staleness checks.
// Snapshot reads use this when no StaleAge is provided.
const defaultStaleAge = 5 * time.Minute

// MetricStateView is one metric's current value, observation time, and quality.
type MetricStateView struct {
	MetricID   string
	Value      float64
	ObservedAt time.Time
	Quality    model.QualityStatus
}

// SnapshotView is the application-layer read model for a CU's real-time state.
// Stale is derived from UpdatedAt and the requested age. Per-metric freshness
// is ObservedAt; callers compare that themselves.
type SnapshotView struct {
	TenantID  string
	CUCode    string
	Metrics   []MetricStateView
	UpdatedAt time.Time
	Stale     bool
}

// snapshotToView converts a domain Snapshot to a SnapshotView.
// metricIDs, when non-empty, keeps only those metrics and preserves that order.
// Missing requested metrics are omitted. Pass staleAge == 0 to skip the CU staleness check.
func snapshotToView(s *model.Snapshot, metricIDs []string, staleAge time.Duration) *SnapshotView {
	return &SnapshotView{
		TenantID:  s.TenantID,
		CUCode:    s.CUCode,
		Metrics:   metricStates(s, metricIDs),
		UpdatedAt: s.UpdatedAt,
		Stale:     staleAge > 0 && s.IsStale(staleAge),
	}
}

func metricStates(s *model.Snapshot, metricIDs []string) []MetricStateView {
	ids := orderedMetricIDs(s.Metrics, metricIDs)
	out := make([]MetricStateView, 0, len(ids))
	for _, id := range ids {
		state := s.Metrics[id]
		out = append(out, MetricStateView{
			MetricID:   state.MetricID,
			Value:      state.Value,
			ObservedAt: state.ObservedAt,
			Quality:    state.Quality,
		})
	}
	return out
}

func orderedMetricIDs(metrics map[string]model.MetricState, filter []string) []string {
	if len(filter) == 0 {
		ids := make([]string, 0, len(metrics))
		for id := range metrics {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids
	}
	out := make([]string, 0, len(filter))
	seen := make(map[string]struct{}, len(filter))
	for _, id := range filter {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		if _, ok := metrics[id]; ok {
			out = append(out, id)
		}
	}
	return out
}
