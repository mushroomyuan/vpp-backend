package model

import "time"

// Snapshot holds the latest known good-quality metric values for a single CU.
//
// It is the real-time "current state" view of a CU, updated on every ingest
// via Apply. Only QualityGood samples are written; bad or uncertain readings
// leave the previous value intact. Snapshots are stored in a fast cache
// (e.g. Redis) and are authoritative for dashboard reads and control decisions.
type Snapshot struct {
	TenantID  string
	CUCode    string
	Metrics   map[string]float64
	UpdatedAt time.Time
}

// NewSnapshot initialises an empty Snapshot for the given CU.
func NewSnapshot(tenantID, cuCode string) *Snapshot {
	return &Snapshot{
		TenantID:  tenantID,
		CUCode:    cuCode,
		Metrics:   make(map[string]float64),
		UpdatedAt: time.Now(),
	}
}

// Apply merges a TelemetryRecord into the snapshot and returns any SOE events
// that were produced.
//
// Domain rules:
//   - Only QualityGood metrics are written; degraded-quality readings are skipped.
//   - A Discrete metric whose value has changed triggers one SOEEvent per change.
//   - Analog metric changes are silently updated (alarm/threshold logic belongs
//     in an application-layer policy, not here).
//   - UpdatedAt is advanced to the record's timestamp even when no metrics changed
//     (e.g. all-bad batch), so staleness detection remains accurate.
func (s *Snapshot) Apply(record *TelemetryRecord) []*SOEEvent {
	var events []*SOEEvent
	for _, m := range record.Metrics {
		if !m.IsGood() {
			continue
		}
		if m.IsDiscrete() {
			if prev, exists := s.Metrics[m.Name]; exists && prev != m.Value {
				events = append(events, NewSOEEvent(
					s.TenantID, s.CUCode, m.Name, prev, m.Value, record.Timestamp,
				))
			}
		}
		s.Metrics[m.Name] = m.Value
	}
	s.UpdatedAt = record.Timestamp
	return events
}

// Get returns the current value for a metric.
// ok is false if the metric has never been recorded in this snapshot.
func (s *Snapshot) Get(metricName string) (value float64, ok bool) {
	value, ok = s.Metrics[metricName]
	return
}

// IsStale returns true if the snapshot has not been updated within maxAge.
// Useful for connection health checks: a CU that stops sending data produces
// a stale snapshot, which should trigger a connectivity alert.
func (s *Snapshot) IsStale(maxAge time.Duration) bool {
	return time.Since(s.UpdatedAt) > maxAge
}
