// Package kafka implements Kafka consumer adapters for the dispatch service.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/dispatch/application/command"
	"github.com/mushroomyuan/vpp-backend/dispatch/domain/model"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	gwEvent "github.com/mushroomyuan/vpp-backend/platform/event/gateway"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// CommandResultConsumerConfig holds Kafka consumer connection parameters.
type CommandResultConsumerConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

// CommandResultConsumer consumes vpp.command.events and drives HandleCommandResult.
//
// Design:
//   - at-least-once: commit only after successful handling
//   - Brokers empty → no-op (Run waits on ctx)
type CommandResultConsumer struct {
	cfg     CommandResultConsumerConfig
	reader  *kafka.Reader
	handler command.HandleCommandResultHandler
}

func NewCommandResultConsumer(
	cfg CommandResultConsumerConfig,
	handler command.HandleCommandResultHandler,
) *CommandResultConsumer {
	if cfg.Topic == "" {
		cfg.Topic = gwEvent.TopicCommandEvents
	}
	if cfg.GroupID == "" {
		cfg.GroupID = "vpp-dispatch-command-events"
	}
	c := &CommandResultConsumer{cfg: cfg, handler: handler}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — command result consumer will not start")
		return c
	}

	c.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		CommitInterval: 0,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-dispatch-consumer] "+msg, args...)
		}),
	})
	logrus.Infof("kafka: command result consumer initialised, brokers=%v topic=%s group=%s",
		cfg.Brokers, cfg.Topic, cfg.GroupID)
	return c
}

// Run starts the consume loop until ctx is cancelled.
func (c *CommandResultConsumer) Run(ctx context.Context) error {
	if c.reader == nil {
		<-ctx.Done()
		return nil
	}

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Transient broker/coordinator errors must not tear down the process.
			logging.Warnf(ctx, logrus.Fields{
				"component": "CommandResultConsumer",
				"error":     err.Error(),
			}, "kafka: fetch message failed, retrying")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		if handleErr := c.handleMessage(ctx, msg); handleErr != nil {
			// Error already logged inside handleMessage with consumer span context.
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			logging.Warnf(ctx, logrus.Fields{
				"component": "CommandResultConsumer",
				"error":     err.Error(),
			}, "kafka: commit failed — message may be reprocessed")
		}
	}
}

// Close shuts down the underlying Kafka reader.
func (c *CommandResultConsumer) Close() error {
	if c.reader == nil {
		return nil
	}
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("kafka command result consumer close: %w", err)
	}
	return nil
}

func (c *CommandResultConsumer) handleMessage(ctx context.Context, msg kafka.Message) (err error) {
	carrier := kafkaHeadersToCarrier(msg.Headers)
	eventType := peekEventType(msg.Value)

	ctx, span := plattelemetry.StartKafkaConsumer(ctx, carrier, plattelemetry.KafkaConsumeInfo{
		Topic:     msg.Topic,
		GroupID:   c.cfg.GroupID,
		Key:       string(msg.Key),
		EventType: eventType,
		Partition: msg.Partition,
		Offset:    msg.Offset,
	})
	defer func() {
		if err != nil {
			logging.Errorf(ctx, logrus.Fields{
				"component": "CommandResultConsumer",
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"error":     err.Error(),
			}, "kafka: message handling failed, skipping commit for retry")
		}
		plattelemetry.EndSpan(span, err)
	}()

	var env platEvent.Envelope[json.RawMessage]
	if err = json.Unmarshal(msg.Value, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	switch env.EventType {
	case gwEvent.TypeCommandCompleted:
		var payload gwEvent.CommandCompletedPayload
		if err = json.Unmarshal(env.Payload, &payload); err != nil {
			return fmt.Errorf("unmarshal CommandCompletedPayload: %w", err)
		}
		result := &model.CommandResult{
			Success:      payload.Success,
			ErrorCode:    payload.ErrorCode,
			ErrorMessage: payload.ErrorMessage,
			AckAt:        payload.AckAt,
		}
		_, err = c.handler.Handle(ctx, command.HandleCommandResult{
			CommandID: payload.CommandID,
			Result:    result,
		})
		return err
	default:
		logging.Debugf(ctx, logrus.Fields{
			"component":  "CommandResultConsumer",
			"event_type": env.EventType,
		}, "kafka: ignoring unknown command event type")
		return nil
	}
}

func kafkaHeadersToCarrier(headers []kafka.Header) plattelemetry.MapCarrier {
	if len(headers) == 0 {
		return nil
	}
	c := make(plattelemetry.MapCarrier, len(headers))
	for _, h := range headers {
		c[h.Key] = string(h.Value)
	}
	return c
}

func peekEventType(value []byte) string {
	var head struct {
		EventType string `json:"event_type"`
	}
	_ = json.Unmarshal(value, &head)
	return head.EventType
}
