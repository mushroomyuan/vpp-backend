package port

import (
	"context"
	"time"

	"github.com/mushroomyuan/vpp-backend/api/contracts"
)

// Quality is the per-metric sample quality from Telemetry.
// A non-good sample is still returned; callers decide whether it can be used.
type Quality string

const (
	QualityUnspecified Quality = "unspecified"
	QualityGood        Quality = "good"
	QualityBad         Quality = "bad"
	QualityUncertain   Quality = "uncertain"
)

// MetricSample is one canonical numeric metric on one CU.
type MetricSample struct {
	MetricID   contracts.MetricID
	Value      float64
	ObservedAt time.Time
	Quality    Quality
}

// CUSnapshot is the latest requested metrics for one CU.
// Stale is the CU-level flag from Telemetry. Each sample keeps its own time and quality.
type CUSnapshot struct {
	CUCode    string
	Metrics   []MetricSample
	UpdatedAt time.Time
	Stale     bool
}

// SnapshotQuery names the CUs and metrics to read. It does not scan a tenant.
// StaleAge 0 asks Telemetry to use its server default.
type SnapshotQuery struct {
	TenantID  string
	CUCodes   []string
	MetricIDs []contracts.MetricID
	StaleAge  time.Duration
}

// TelemetryPort reads current canonical metric state.
type TelemetryPort interface {
	GetSnapshots(ctx context.Context, query SnapshotQuery) ([]CUSnapshot, error)
}
