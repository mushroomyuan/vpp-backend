// Package kafka provides a Kafka-backed implementation of
// port.ResourceEventPublisher. It wraps a segmentio/kafka-go Writer and
// gracefully degrades to a no-op when no brokers are configured, mirroring the
// same pattern used by the telemetry service's SOE publisher.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"github.com/mushroomyuan/vpp-backend/resource/domain/port"
)

// Config holds Kafka producer connection parameters for the resource event bus.
type Config struct {
	Brokers []string
	Topic   string
}

// EventPublisher publishes resource lifecycle events to a Kafka topic.
//
// When Brokers is empty (e.g. Kafka not yet provisioned), the publisher
// degrades gracefully: events are logged at Debug level and silently dropped.
// Replace config.Brokers with real addresses to activate the real producer
// without any other code change.
type EventPublisher struct {
	cfg    Config
	writer *kafka.Writer // nil when Brokers is empty
}

// NewEventPublisher constructs the publisher. If cfg.Brokers is empty a no-op
// (log-only) publisher is returned; otherwise a real Kafka writer is created.
func NewEventPublisher(cfg Config) *EventPublisher {
	p := &EventPublisher{cfg: cfg}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — resource events will be dropped")
		return p
	}

	p.writer = &kafka.Writer{
		Addr:  kafka.TCP(cfg.Brokers...),
		Topic: cfg.Topic,
		// Hash-partition by tenant:resource so that events for the same resource
		// are always routed to the same partition and arrive in order.
		Balancer: &kafka.Hash{},
		// Async = true: WriteMessages queues the message and returns immediately.
		// The background writer batches and flushes to Kafka. This prevents Kafka
		// latency from blocking command handlers.
		// Errors are surfaced via ErrorLogger; callers treat Publish as best-effort.
		Async:                  true,
		AllowAutoTopicCreation: true,
		WriteTimeout:           5 * time.Second,
		ReadTimeout:            5 * time.Second,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-resource] "+msg, args...)
		}),
	}
	logrus.Infof("kafka: resource event publisher connected to %v, topic=%s", cfg.Brokers, cfg.Topic)
	return p
}

// Publish serialises the event into an Envelope and writes it to Kafka.
// When Kafka is not configured the event is logged at Debug level and dropped.
// Async write errors are reported by the ErrorLogger on the kafka.Writer.
func (p *EventPublisher) Publish(ctx context.Context, event port.ResourceEvent) error {
	if p.writer == nil {
		logging.Debugf(ctx, logrus.Fields{
			"component":   "ResourceEventPublisher",
			"event_type":  event.EventType,
			"tenant_id":   event.TenantID,
			"resource_id": event.ResourceID,
		}, "kafka not configured — resource event dropped")
		return nil
	}

	eventID := idgen.Must()

	payloadBytes, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal resource event payload: %w", err)
	}

	envelope := platEvent.Envelope[json.RawMessage]{
		EventID:    eventID,
		EventType:  event.EventType,
		Version:    "v1",
		TenantID:   event.TenantID,
		OccurredAt: time.Now(),
		Payload:    json.RawMessage(payloadBytes),
	}

	msgBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal resource event envelope: %w", err)
	}

	// Partition key: "tenantID:resourceID" ensures ordering per resource.
	key := event.TenantID
	if event.ResourceID != "" {
		key = fmt.Sprintf("%s:%s", event.TenantID, event.ResourceID)
	}

	ctx, span := plattelemetry.StartKafkaProducer(ctx, plattelemetry.KafkaPublishInfo{
		Topic:     p.cfg.Topic,
		Key:       key,
		EventType: event.EventType,
	})
	writeErr := p.writer.WriteMessages(ctx, kafka.Message{
		Key:     []byte(key),
		Value:   msgBytes,
		Headers: mapToKafkaHeaders(plattelemetry.InjectToMap(ctx)),
	})
	plattelemetry.EndSpan(span, writeErr)
	return writeErr
}

func mapToKafkaHeaders(c plattelemetry.MapCarrier) []kafka.Header {
	if len(c) == 0 {
		return nil
	}
	out := make([]kafka.Header, 0, len(c))
	for k, v := range c {
		out = append(out, kafka.Header{Key: k, Value: []byte(v)})
	}
	return out
}

// Close flushes any buffered messages and closes the underlying Kafka writer.
// Must be called during graceful shutdown.
func (p *EventPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("kafka resource event writer close: %w", err)
	}
	return nil
}

var _ port.ResourceEventPublisher = (*EventPublisher)(nil)
