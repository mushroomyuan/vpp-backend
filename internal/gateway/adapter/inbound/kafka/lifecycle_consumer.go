// Package kafka implements Kafka consumer adapters for the gateway service.
// The LifecycleConsumer subscribes to vpp.resource.events and translates
// resource lifecycle events into gateway mapping state changes.
package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/gateway/application/command"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	resEvent "github.com/mushroomyuan/vpp-backend/platform/event/resource"
)

// LifecycleConsumerConfig holds the Kafka consumer connection parameters.
type LifecycleConsumerConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

// LifecycleConsumer consumes resource lifecycle events and drives gateway
// mapping state changes.
//
// Design decisions:
//   - at-least-once delivery: offset is committed only after successful handling.
//   - On handler failure the message is NOT committed so the consumer group
//     will retry on the next poll.
//   - When Brokers is empty the consumer is not started (no-op degradation).
type LifecycleConsumer struct {
	cfg     LifecycleConsumerConfig
	reader  *kafka.Reader
	handler command.DisableMappingByCUCodeHandler
}

// NewLifecycleConsumer constructs the consumer. If cfg.Brokers is empty
// the returned consumer will be a no-op (Run returns immediately).
func NewLifecycleConsumer(
	cfg LifecycleConsumerConfig,
	handler command.DisableMappingByCUCodeHandler,
) *LifecycleConsumer {
	c := &LifecycleConsumer{cfg: cfg, handler: handler}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — lifecycle consumer will not start")
		return c
	}

	c.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.Brokers,
		Topic:   cfg.Topic,
		GroupID: cfg.GroupID,
		// CommitInterval 0 means explicit manual commit after each message.
		CommitInterval: 0,
		// Errors are surfaced via ErrorLogger.
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-gateway-consumer] "+msg, args...)
		}),
	})

	logrus.Infof("kafka: lifecycle consumer initialised, brokers=%v topic=%s group=%s",
		cfg.Brokers, cfg.Topic, cfg.GroupID)
	return c
}

// Run starts the consume loop. It blocks until ctx is cancelled.
// Designed to be launched in a goroutine / errgroup.
func (c *LifecycleConsumer) Run(ctx context.Context) error {
	if c.reader == nil {
		// No-op mode — wait for context cancellation and exit cleanly.
		<-ctx.Done()
		return nil
	}

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				// Normal shutdown — context cancelled.
				return nil
			}
			// Transient broker/coordinator errors (e.g. offsets topic not ready)
			// must not tear down the whole process via errgroup.
			logrus.WithError(err).Warn("kafka: fetch message failed, retrying")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}

		if handleErr := c.handleMessage(ctx, msg); handleErr != nil {
			// Log and do NOT commit — consumer group will retry.
			logrus.WithError(handleErr).WithFields(logrus.Fields{
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
			}).Error("kafka: message handling failed, skipping commit for retry")
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			logrus.WithError(err).Warn("kafka: commit failed — message may be reprocessed")
		}
	}
}

// Close shuts down the underlying Kafka reader.
func (c *LifecycleConsumer) Close() error {
	if c.reader == nil {
		return nil
	}
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("kafka lifecycle consumer close: %w", err)
	}
	return nil
}

// handleMessage deserialises the envelope and dispatches to the appropriate
// domain handler based on EventType.
func (c *LifecycleConsumer) handleMessage(ctx context.Context, msg kafka.Message) error {
	// First pass: deserialise the envelope header only (payload as raw JSON).
	var env platEvent.Envelope[json.RawMessage]
	if err := json.Unmarshal(msg.Value, &env); err != nil {
		// Unparseable message — log and skip (commit will happen after return nil).
		logrus.WithError(err).WithField("offset", msg.Offset).
			Warn("kafka: failed to deserialise envelope, skipping message")
		return nil
	}

	switch env.EventType {
	case resEvent.TypeResourceDeleted:
		return c.handleResourceDeleted(ctx, env)

	case resEvent.TypeLifecycleChanged:
		return c.handleLifecycleChanged(ctx, env)

	default:
		// We only care about the two event types above; all others are silently ignored.
		return nil
	}
}

func (c *LifecycleConsumer) handleResourceDeleted(
	ctx context.Context,
	env platEvent.Envelope[json.RawMessage],
) error {
	var payload resEvent.ResourceDeletedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		logrus.WithError(err).WithField("event_id", env.EventID).
			Warn("kafka: failed to deserialise ResourceDeletedPayload, skipping")
		return nil
	}

	tenantID := env.TenantID
	if tenantID == "" {
		tenantID = payload.TenantID
	}

	_, err := c.handler.Handle(ctx, command.DisableMappingByCUCode{
		TenantID: tenantID,
		CUCode:   payload.ResourceID,
	})
	if err != nil {
		return fmt.Errorf("disable mapping on resource.deleted (resource_id=%s): %w", payload.ResourceID, err)
	}
	return nil
}

func (c *LifecycleConsumer) handleLifecycleChanged(
	ctx context.Context,
	env platEvent.Envelope[json.RawMessage],
) error {
	var payload resEvent.LifecycleChangedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		logrus.WithError(err).WithField("event_id", env.EventID).
			Warn("kafka: failed to deserialise LifecycleChangedPayload, skipping")
		return nil
	}

	// Only react to disabling/archiving lifecycle transitions.
	if !isInactiveStatus(payload.Status) {
		return nil
	}

	tenantID := env.TenantID
	if tenantID == "" {
		tenantID = payload.TenantID
	}

	_, err := c.handler.Handle(ctx, command.DisableMappingByCUCode{
		TenantID: tenantID,
		CUCode:   payload.ResourceID,
	})
	if err != nil {
		return fmt.Errorf("disable mapping on lifecycle.changed (resource_id=%s, status=%s): %w",
			payload.ResourceID, payload.Status, err)
	}
	return nil
}

// isInactiveStatus returns true for lifecycle states that should cause the
// gateway mapping to be disabled.
func isInactiveStatus(status string) bool {
	switch status {
	case "disabled", "archived":
		return true
	default:
		return false
	}
}
