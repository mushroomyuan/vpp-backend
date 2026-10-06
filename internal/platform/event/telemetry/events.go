package telemetry

import "time"

// SOE kinds. Alarm rules match one kind each. A quality or staleness fact
// must not be published as a discrete change.
const (
	KindDiscreteChange   = "discrete_change"
	KindQualityBad       = "quality_bad"
	KindQualityUncertain = "quality_uncertain"
	KindStale            = "stale"
	KindRecovery         = "recovery"
)

// Quality values follow the IEC 60870-5 / OPC-UA convention used by telemetry.
const (
	QualityGood      = "GOOD"
	QualityBad       = "BAD"
	QualityUncertain = "UNCERTAIN"
)

// SOEPayload is the JSON body published to TopicSOEEvents.
//
// Field names are the v2 wire contract and must stay identical to the
// telemetry producer's soePayload. metric_id is a canonical contract ID.
// observed_at, quality, and value describe that sample. previous_value is
// set when the event is a transition from a known sample.
type SOEPayload struct {
	SchemaVersion string    `json:"schema_version"`
	TenantID      string    `json:"tenant_id"`
	CUCode        string    `json:"cu_code"`
	MetricID      string    `json:"metric_id"`
	Kind          string    `json:"kind"`
	Quality       string    `json:"quality"`
	Value         float64   `json:"value"`
	PreviousValue *float64  `json:"previous_value,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
}
