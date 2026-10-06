package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	telEvent "github.com/mushroomyuan/vpp-backend/platform/event/telemetry"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/model"
	"github.com/mushroomyuan/vpp-backend/telemetry/domain/port"
)

// Config holds Kafka producer connection parameters.
type Config struct {
	Brokers []string
	Topic   string
}

// EventPublisher publishes SOE events to a Kafka topic.
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
		logrus.Warn("kafka: no brokers configured — SOE events will be dropped")
		return p
	}

	p.writer = &kafka.Writer{
		Addr:  kafka.TCP(cfg.Brokers...),
		Topic: cfg.Topic,
		// Hash-partition by tenant:cu so that events for the same CU are
		// always routed to the same partition and arrive in order.
		Balancer: &kafka.Hash{},
		// Async = true: WriteMessages queues the message and returns
		// immediately. The background writer batches and flushes to Kafka.
		// This prevents Kafka latency from blocking the IngestTelemetry path.
		// Errors are surfaced via ErrorLogger below; callers already treat
		// PublishSOE as best-effort (errors are silently ignored).
		Async:                  true,
		AllowAutoTopicCreation: true,
		WriteTimeout:           5 * time.Second,
		ReadTimeout:            5 * time.Second,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-soe] "+msg, args...)
		}),
	}
	logrus.Infof("kafka: SOE publisher connected to %v, topic=%s", cfg.Brokers, cfg.Topic)
	return p
}

// soePayload is the JSON body sent to Kafka. Tags must stay identical to
// platform/event/telemetry.SOEPayload. The producer marshals this local type
// so the wire format cannot drift from a platform change without an explicit
// edit here.
type soePayload struct {
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

// PublishSOE publishes one SOE event. When Kafka is not configured it logs at
// Debug level and returns nil. Async write errors are logged by ErrorLogger.
func (p *EventPublisher) PublishSOE(ctx context.Context, event *model.SOEEvent) error {
	if p.writer == nil {
		logging.Debugf(ctx, logrus.Fields{
			"component": "SOEEventPublisher",
			"tenant_id": event.TenantID,
			"cu_code":   event.CUCode,
			"metric_id": event.MetricID,
			"kind":      event.Kind,
			"quality":   event.Quality,
			"value":     event.Value,
		}, "kafka not configured — SOE event dropped")
		return nil
	}

	payload, err := json.Marshal(soePayload{
		SchemaVersion: telEvent.SchemaVersionV2,
		TenantID:      event.TenantID,
		CUCode:        event.CUCode,
		MetricID:      event.MetricID,
		Kind:          event.Kind,
		Quality:       string(event.Quality),
		Value:         event.Value,
		PreviousValue: event.PreviousValue,
		ObservedAt:    event.ObservedAt,
	})
	if err != nil {
		return fmt.Errorf("marshal SOE event: %w", err)
	}

	// Partition key = "tenant_id:cu_code" ensures ordering per CU.
	key := fmt.Sprintf("%s:%s", event.TenantID, event.CUCode)
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: payload,
	})
}

// Close flushes any buffered messages and closes the underlying Kafka writer.
// Must be called during graceful shutdown.
func (p *EventPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("kafka writer close: %w", err)
	}
	return nil
}

var _ port.EventPublisher = (*EventPublisher)(nil)
