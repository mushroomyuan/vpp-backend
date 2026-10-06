package model

import "time"

// MetricState is the latest sample of one canonical metric on a CU.
// A later sample always replaces this state, including when its quality is
// bad or uncertain, so an older good value cannot hide a current quality fault.
type MetricState struct {
	MetricID   string
	Value      float64
	ObservedAt time.Time
	Quality    QualityStatus
}

// Snapshot holds the latest per-metric state for a single CU.
//
// UpdatedAt is the timestamp of the latest applied ingest. Each metric keeps
// its own ObservedAt and Quality. Snapshots are stored in Redis and are the
// real-time state Decision reads.
type Snapshot struct {
	TenantID  string
	CUCode    string
	Metrics   map[string]MetricState
	UpdatedAt time.Time
}

// NewSnapshot initialises an empty Snapshot for the given CU.
func NewSnapshot(tenantID, cuCode string) *Snapshot {
	return &Snapshot{
		TenantID:  tenantID,
		CUCode:    cuCode,
		Metrics:   make(map[string]MetricState),
		UpdatedAt: time.Now(),
	}
}

// Apply merges a TelemetryRecord into the snapshot and returns SOE events.
//
// Domain rules:
//   - Every sample, including bad and uncertain quality, replaces the previous
//     state for that metric. The previous good value is not retained.
//   - A Discrete metric emits discrete_change only when both samples are
//     QualityGood, the value changed, and the gap is within staleAge.
//   - Entering BAD or UNCERTAIN emits that quality kind. Returning to GOOD,
//     or a fresh GOOD sample after a stale gap, emits recovery.
//   - When the previous observation is older than staleAge, that observation
//     is emitted as stale. It is not a healthy discrete change.
//   - staleAge <= 0 disables gap detection. A zero previous ObservedAt is not
//     treated as stale, because its age is unknown.
//   - UpdatedAt advances to the record timestamp even when the batch has no
//     good samples, so CU-level staleness still tracks the latest ingest.
func (s *Snapshot) Apply(record *TelemetryRecord, staleAge time.Duration) []*SOEEvent {
	if s.Metrics == nil {
		s.Metrics = make(map[string]MetricState)
	}
	var events []*SOEEvent
	for _, m := range record.Metrics {
		prev, exists := s.Metrics[m.MetricID]
		gapStale := exists && sampleWentStale(prev, record.Timestamp, staleAge)
		if gapStale {
			events = append(events, NewSOEEvent(
				s.TenantID, s.CUCode, m.MetricID, SOEKindStale,
				prev.Quality, prev.Value, nil, prev.ObservedAt,
			))
		}
		events = append(events, transitionEvents(s.TenantID, s.CUCode, m, prev, exists, gapStale, record.Timestamp)...)
		s.Metrics[m.MetricID] = MetricState{
			MetricID:   m.MetricID,
			Value:      m.Value,
			ObservedAt: record.Timestamp,
			Quality:    m.Quality,
		}
	}
	s.UpdatedAt = record.Timestamp
	return events
}

func sampleWentStale(prev MetricState, at time.Time, staleAge time.Duration) bool {
	if staleAge <= 0 || prev.ObservedAt.IsZero() || !at.After(prev.ObservedAt) {
		return false
	}
	return at.Sub(prev.ObservedAt) > staleAge
}

func transitionEvents(tenantID, cuCode string, m Metric, prev MetricState, exists, gapStale bool, at time.Time) []*SOEEvent {
	var previous *float64
	if exists {
		previous = &prev.Value
	}
	switch m.Quality {
	case QualityBad:
		if exists && prev.Quality == QualityBad {
			return nil
		}
		return []*SOEEvent{NewSOEEvent(tenantID, cuCode, m.MetricID, SOEKindQualityBad, QualityBad, m.Value, previous, at)}
	case QualityUncertain:
		if exists && prev.Quality == QualityUncertain {
			return nil
		}
		return []*SOEEvent{NewSOEEvent(tenantID, cuCode, m.MetricID, SOEKindQualityUncertain, QualityUncertain, m.Value, previous, at)}
	case QualityGood:
		var events []*SOEEvent
		recovered := exists && (prev.Quality == QualityBad || prev.Quality == QualityUncertain || gapStale)
		if recovered {
			events = append(events, NewSOEEvent(tenantID, cuCode, m.MetricID, SOEKindRecovery, QualityGood, m.Value, previous, at))
		}
		if m.IsDiscrete() && exists && prev.Quality == QualityGood && prev.Value != m.Value && !gapStale {
			events = append(events, NewSOEEvent(tenantID, cuCode, m.MetricID, SOEKindDiscreteChange, QualityGood, m.Value, previous, at))
		}
		return events
	default:
		return nil
	}
}

// Get returns the current state for a metric.
// ok is false if the metric has never been recorded in this snapshot.
func (s *Snapshot) Get(metricID string) (MetricState, bool) {
	state, ok := s.Metrics[metricID]
	return state, ok
}

// IsStale returns true if the snapshot has not been updated within maxAge.
// Per-metric freshness uses MetricState.ObservedAt; this check is the CU heartbeat.
func (s *Snapshot) IsStale(maxAge time.Duration) bool {
	return time.Since(s.UpdatedAt) > maxAge
}
