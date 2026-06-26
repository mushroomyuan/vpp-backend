package kafkapub

import (
	"context"
	"fmt"

	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// Config holds Kafka producer connection parameters.
// Populated here for future use; no fields are required for the stub.
type Config struct {
	Brokers []string
	Topic   string
}

// EventPublisher is a stub implementation of port.EventPublisher.
// Replace the body of PublishSOE with a real kafka-go or sarama producer
// once the Kafka infrastructure is provisioned.
type EventPublisher struct {
	cfg Config
}

func NewEventPublisher(cfg Config) *EventPublisher {
	return &EventPublisher{cfg: cfg}
}

// PublishSOE is a no-op placeholder.
// TODO: implement with kafka-go writer once broker is available.
func (p *EventPublisher) PublishSOE(_ context.Context, event *model.SOEEvent) error {
	// Intentionally not implemented yet.
	// When ready, marshal event to JSON and produce to p.cfg.Topic.
	_ = fmt.Sprintf("TODO: publish SOE to %s: %+v", p.cfg.Topic, event)
	return nil
}

var _ port.EventPublisher = (*EventPublisher)(nil)
