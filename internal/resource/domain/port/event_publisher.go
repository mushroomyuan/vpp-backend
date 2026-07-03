package port

import "context"

// ResourceEvent carries the data for a single resource lifecycle event to be
// published to the event bus.
type ResourceEvent struct {
	// EventType is one of the resource.TypeXxx constants from
	// platform/event/resource/topic.go, e.g. "resource.cu.created".
	EventType string

	// TenantID is the owning tenant. Always required.
	TenantID string

	// ResourceID is the primary entity involved (site_id, asset_id, cu_id,
	// point_id, job_id, …). Used as a component of the Kafka partition key to
	// guarantee ordering of events for the same resource.
	ResourceID string

	// Payload is a type-specific struct from platform/event/resource/events.go.
	// The publisher implementation is responsible for JSON-marshalling it.
	Payload any
}

// ResourceEventPublisher is the outbound port for resource lifecycle events.
//
// Application-layer command handlers depend only on this interface; the Kafka
// producer (or a future Outbox implementation) satisfies it from the
// infrastructure / adapter side. Swapping "direct Kafka" for "write Outbox
// table" requires only replacing the implementation injected at startup —
// no business code changes.
//
// All Publish calls are best-effort: callers MUST log errors but MUST NOT
// propagate them as command failures.
type ResourceEventPublisher interface {
	Publish(ctx context.Context, event ResourceEvent) error
	Close() error
}
