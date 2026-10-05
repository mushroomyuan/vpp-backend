package model

// ForecastTarget binds one CU metric to one Predictor. TenantID lives on
// the target itself rather than a service-level tenant-ids list: a target
// is configured against a specific tenant's asset, and CUCode is already
// globally unique (Resource CU UUID), so a cross-product of tenants ×
// targets would issue QueryAggregation calls that cannot succeed.
//
// This is a plain struct rather than an interface
// because both algorithms share TenantID/CUCode/MetricName and only
// differ by one sidecar parameter. That matches dispatch.CommandValue's
// "few branches, simple scalars, not expected to grow" case (discussion
// §3.5), not PointTarget/AggregateTarget's structurally diverging
// payloads.
//
// Targets are bound explicitly. Point.PointKey has no project-wide role
// or tag, so v1 cannot auto-discover "which metrics should be forecast".
type ForecastTarget struct {
	Enabled    bool
	TenantID   string
	CUCode     string
	MetricName string
	Algorithm  AlgorithmID

	// MovingAverageWindow is the number of most-recent history buckets
	// averaged when Algorithm is moving_average. Unused otherwise.
	MovingAverageWindow int
	// SamePeriodLookbackDays is how many prior days at the same clock
	// time are averaged when Algorithm is same_period_prior. Unused
	// otherwise. v1 uses the arithmetic mean, not the median.
	SamePeriodLookbackDays int
}
