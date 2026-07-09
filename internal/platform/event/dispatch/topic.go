// Package dispatch defines the Kafka topic, event-type constants, and version
// for DispatchTask lifecycle events published by the vpp-dispatch service.
package dispatch

const (
	// TopicDispatchEvents is the Kafka topic for task lifecycle events.
	TopicDispatchEvents = "vpp.dispatch.events"

	// VersionV1 is the initial payload schema version.
	VersionV1 = "v1"

	TypeTaskStarted   = "task.started"
	TypeTaskCompleted = "task.completed"
	TypeTaskFailed    = "task.failed"
)
