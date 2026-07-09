// Package kafka provides a Kafka-backed implementation of
// domain/port.TaskEventPublisher for DispatchTask lifecycle events.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/port"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
)

// Config holds Kafka producer connection parameters for dispatch task events.
type Config struct {
	Brokers []string
	Topic   string // default: vpp.dispatch.events
}

// EventPublisher publishes task lifecycle events to Kafka.
// When Brokers is empty, events are logged at Debug and dropped (no-op).
type EventPublisher struct {
	cfg    Config
	writer *kafka.Writer
}

var _ port.TaskEventPublisher = (*EventPublisher)(nil)

func NewEventPublisher(cfg Config) *EventPublisher {
	if cfg.Topic == "" {
		cfg.Topic = dispEvent.TopicDispatchEvents
	}
	p := &EventPublisher{cfg: cfg}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — dispatch task events will be dropped")
		return p
	}
	p.writer = &kafka.Writer{
		Addr:                   kafka.TCP(cfg.Brokers...),
		Topic:                  cfg.Topic,
		Balancer:               &kafka.Hash{},
		Async:                  true,
		AllowAutoTopicCreation: true,
		WriteTimeout:           5 * time.Second,
		ReadTimeout:            5 * time.Second,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-dispatch] "+msg, args...)
		}),
	}
	logrus.Infof("kafka: dispatch event publisher connected to %v, topic=%s", cfg.Brokers, cfg.Topic)
	return p
}

func (p *EventPublisher) PublishTaskStarted(ctx context.Context, task *model.DispatchTask) error {
	return p.publish(ctx, dispEvent.TypeTaskStarted, task)
}

func (p *EventPublisher) PublishTaskCompleted(ctx context.Context, task *model.DispatchTask) error {
	return p.publish(ctx, dispEvent.TypeTaskCompleted, task)
}

func (p *EventPublisher) PublishTaskFailed(ctx context.Context, task *model.DispatchTask) error {
	return p.publish(ctx, dispEvent.TypeTaskFailed, task)
}

func (p *EventPublisher) publish(ctx context.Context, eventType string, task *model.DispatchTask) error {
	payload := dispEvent.TaskLifecyclePayload{
		TaskID:   task.ID,
		TenantID: task.TenantID,
		Name:     task.Name,
		Status:   string(task.Status),
	}

	if p.writer == nil {
		logrus.WithFields(logrus.Fields{
			"event_type": eventType,
			"tenant_id":  task.TenantID,
			"task_id":    task.ID,
		}).Debug("kafka not configured — dispatch event dropped")
		return nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal task event payload: %w", err)
	}

	envelope := platEvent.Envelope[json.RawMessage]{
		EventID:    idgen.Must(),
		EventType:  eventType,
		Version:    dispEvent.VersionV1,
		TenantID:   task.TenantID,
		OccurredAt: time.Now(),
		Payload:    json.RawMessage(payloadBytes),
	}
	msgBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal task event envelope: %w", err)
	}

	key := fmt.Sprintf("%s:%s", task.TenantID, task.ID)
	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: msgBytes,
	})
}

// Close flushes buffered messages and closes the writer.
func (p *EventPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("kafka dispatch event publisher close: %w", err)
	}
	return nil
}
