// Package gateway defines the Kafka topic, event-type constants, and version
// for command execution events published by the vpp-gateway service and
// consumed by vpp-dispatch.
package gateway

const (
	// TopicCommandEvents is the Kafka topic for command completion callbacks.
	TopicCommandEvents = "vpp.command.events"

	// VersionV1 is the initial payload schema version.
	VersionV1 = "v1"

	// TypeCommandCompleted is published when a control command reaches a
	// terminal outcome (success or failure) at the gateway/EMS boundary.
	TypeCommandCompleted = "command.completed"
)
