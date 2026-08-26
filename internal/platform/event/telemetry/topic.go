// Package telemetry defines the Kafka topic and wire payload for discrete
// metric state-change (SOE) events published by vpp-telemetry.
//
// v1 messages are flat JSON (SOEPayload), not event.Envelope. The producer is
// unchanged; consumers MUST unmarshal the Kafka value into SOEPayload directly.
// Do not wrap these events in Envelope without a coordinated producer change.
package telemetry

const (
	// TopicSOEEvents is the Kafka topic for discrete metric state-change events.
	TopicSOEEvents = "vpp.soe.events"

	// VersionV1 is the documented payload schema version. It is not a JSON
	// field on the v1 wire (there is no Envelope); consumers treat every
	// message on TopicSOEEvents as this schema.
	VersionV1 = "v1"
)
