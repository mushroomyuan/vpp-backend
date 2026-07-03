package event

import "time"

// Envelope is the common wire-format wrapper for all domain events published to
// Kafka. It carries routing metadata alongside a strongly-typed payload so that
// consumers can inspect headers without deserialising the payload first.
//
// Usage: instantiate with a concrete payload type, then JSON-marshal the whole
// struct for the Kafka message body.
//
//	env := event.Envelope[resource.CUCreatedPayload]{
//	    EventID:    idgen.Must(),
//	    EventType:  resource.TypeCUCreated,
//	    Version:    resource.VersionV1,
//	    TenantID:   tenantID,
//	    OccurredAt: time.Now(),
//	    Payload:    resource.CUCreatedPayload{...},
//	}
type Envelope[T any] struct {
	EventID    string    `json:"event_id"`
	EventType  string    `json:"event_type"`
	Version    string    `json:"version"`
	TenantID   string    `json:"tenant_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Payload    T         `json:"payload"`
}
