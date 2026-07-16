// Package kafka provides Kafka-backed outbound adapters for the gateway service.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/gateway/domain/port"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	gwEvent "github.com/mushroomyuan/vpp-backend/platform/event/gateway"
	"github.com/mushroomyuan/vpp-backend/platform/idgen"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// CommandEventPublisherConfig holds Kafka producer parameters for command events.
type CommandEventPublisherConfig struct {
	Brokers []string
	Topic   string // default: vpp.command.events
}

// CommandEventPublisher publishes CommandCompleted events to Kafka.
// When Brokers is empty, events are logged at Debug and dropped (no-op).
type CommandEventPublisher struct {
	cfg    CommandEventPublisherConfig
	writer *kafka.Writer
}

var _ port.CommandEventPublisher = (*CommandEventPublisher)(nil)

func NewCommandEventPublisher(cfg CommandEventPublisherConfig) *CommandEventPublisher {
	if cfg.Topic == "" {
		cfg.Topic = gwEvent.TopicCommandEvents
	}
	p := &CommandEventPublisher{cfg: cfg}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — command events will be dropped")
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
			logrus.Errorf("[kafka-gateway-command] "+msg, args...)
		}),
	}
	logrus.Infof("kafka: command event publisher connected to %v, topic=%s", cfg.Brokers, cfg.Topic)
	return p
}

func (p *CommandEventPublisher) PublishCommandCompleted(
	ctx context.Context,
	event port.CommandCompletedEvent,
) error {
	payload := gwEvent.CommandCompletedPayload{
		TenantID:     event.TenantID,
		CommandID:    event.CommandID,
		CUCode:       event.CUCode,
		Success:      event.Success,
		ErrorCode:    event.ErrorCode,
		ErrorMessage: event.ErrorMessage,
		AckAt:        event.AckAt,
	}

	if p.writer == nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{
			"component":  "CommandEventPublisher",
			"command_id": event.CommandID,
			"tenant_id":  event.TenantID,
			"success":    event.Success,
		}).Debug("kafka not configured — command event dropped")
		return nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal command completed payload: %w", err)
	}

	envelope := platEvent.Envelope[json.RawMessage]{
		EventID:    idgen.Must(),
		EventType:  gwEvent.TypeCommandCompleted,
		Version:    gwEvent.VersionV1,
		TenantID:   event.TenantID,
		OccurredAt: time.Now(),
		Payload:    json.RawMessage(payloadBytes),
	}
	msgBytes, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal command completed envelope: %w", err)
	}

	key := fmt.Sprintf("%s:%s", event.TenantID, event.CommandID)

	ctx, span := plattelemetry.StartKafkaProducer(ctx, plattelemetry.KafkaPublishInfo{
		Topic:     p.cfg.Topic,
		Key:       key,
		EventType: gwEvent.TypeCommandCompleted,
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

// Close flushes buffered messages and closes the writer.
func (p *CommandEventPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("kafka command event publisher close: %w", err)
	}
	return nil
}
