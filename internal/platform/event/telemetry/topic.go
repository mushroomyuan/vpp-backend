// Package telemetry defines the Kafka topic and wire payload for SOE events
// published by vpp-telemetry and consumed by vpp-alarm.
//
// Messages are flat JSON (SOEPayload), not event.Envelope. schema_version is
// a field on the payload. Consumers accept only SchemaVersionV2 and do not
// dual-read the retired metric_name shape.
package telemetry

const (
	// TopicSOEEvents is the Kafka topic for canonical SOE events.
	TopicSOEEvents = "vpp.soe.events"

	// SchemaVersionV2 is the payload schema. The metric identity is a
	// canonical metric_id. v1 (metric_name, old_value, new_value, occurred_at)
	// is retired; there is no production data and no alias read.
	SchemaVersionV2 = "v2"
)
