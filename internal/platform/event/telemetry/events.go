package telemetry

import "time"

// SOEPayload is the JSON body published to TopicSOEEvents.
//
// Field names and types are a v1 wire contract: they must stay identical to
// the telemetry producer's soePayload. Changing a tag here without changing
// the producer would silently drop or mis-parse events.
type SOEPayload struct {
	TenantID   string    `json:"tenant_id"`
	CUCode     string    `json:"cu_code"`
	MetricName string    `json:"metric_name"`
	OldValue   float64   `json:"old_value"`
	NewValue   float64   `json:"new_value"`
	OccurredAt time.Time `json:"occurred_at"`
}
