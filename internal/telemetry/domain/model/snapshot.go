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

// Apply merges a TelemetryRecord into the snapshot and returns any SOE events
// that were produced.
//
// Domain rules:
//   - Every sample, including bad and uncertain quality, replaces the previous
//     state for that metric. The previous good value is not retained.
//   - A Discrete metric emits one SOEEvent only when both the previous and the
//     new sample are QualityGood and the value changed.
//   - UpdatedAt advances to the record timestamp even when the batch is empty
//     of good samples, so CU-level staleness still tracks the latest ingest.
func (s *Snapshot) Apply(record *TelemetryRecord) []*SOEEvent {
	if s.Metrics == nil {
		s.Metrics = make(map[string]MetricState)
	}
	var events []*SOEEvent
	for _, m := range record.Metrics {
		prev, exists := s.Metrics[m.MetricID]
		if m.IsDiscrete() && m.IsGood() && exists && prev.Quality == QualityGood && prev.Value != m.Value {
			events = append(events, NewSOEEvent(
				s.TenantID, s.CUCode, m.MetricID, prev.Value, m.Value, record.Timestamp,
			))
		}
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
