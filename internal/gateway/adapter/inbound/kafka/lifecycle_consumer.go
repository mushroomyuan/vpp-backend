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
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

// LifecycleConsumerConfig holds the Kafka consumer connection parameters.
type LifecycleConsumerConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

// BindingInvalidator drops cached point bindings. Event handling must not
// load a replacement; the next reader lists the CU from Resource.
type BindingInvalidator interface {
	InvalidateCU(tenantID, cuID string)
	InvalidatePoint(tenantID, pointID string)
	InvalidateTenant(tenantID string)
}

// LifecycleConsumer consumes resource lifecycle events and drives gateway
// mapping state changes. Point and CU events also drop the binding cache.
//
// Design decisions:
//   - at-least-once delivery: offset is committed only after successful handling.
//   - On handler failure the message is NOT committed so the consumer group
//     will retry on the next poll.
//   - When Brokers is empty the consumer is not started (no-op degradation).
type LifecycleConsumer struct {
	cfg      LifecycleConsumerConfig
	reader   *kafka.Reader
	handler  command.DisableMappingByCUCodeHandler
	bindings BindingInvalidator
}

// NewLifecycleConsumer constructs the consumer. If cfg.Brokers is empty
// the returned consumer will be a no-op (Run returns immediately).
func NewLifecycleConsumer(
	cfg LifecycleConsumerConfig,
	handler command.DisableMappingByCUCodeHandler,
	bindings BindingInvalidator,
) *LifecycleConsumer {
	c := &LifecycleConsumer{cfg: cfg, handler: handler, bindings: bindings}
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
			logging.Warnf(ctx, logrus.Fields{
				"component": "LifecycleConsumer",
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
				"component": "LifecycleConsumer",
				"error":     err.Error(),
			}, "kafka: commit failed — message may be reprocessed")
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
func (c *LifecycleConsumer) handleMessage(ctx context.Context, msg kafka.Message) (err error) {
	carrier := kafkaHeadersToCarrier(msg.Headers)

	// Peek event type for span attrs (best-effort; full parse follows).
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
				"component": "LifecycleConsumer",
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"error":     err.Error(),
			}, "kafka: message handling failed, skipping commit for retry")
		}
		plattelemetry.EndSpan(span, err)
	}()

	var env platEvent.Envelope[json.RawMessage]
	if unmarshalErr := json.Unmarshal(msg.Value, &env); unmarshalErr != nil {
		// Unparseable message — log and skip (commit will happen after return nil).
		logging.Warnf(ctx, logrus.Fields{
			"component": "LifecycleConsumer",
			"offset":    msg.Offset,
			"error":     unmarshalErr.Error(),
		}, "kafka: failed to deserialise envelope, skipping message")
		return nil
	}

	switch env.EventType {
	case resEvent.TypeResourceDeleted:
		return c.handleResourceDeleted(ctx, env)

	case resEvent.TypeLifecycleChanged:
		return c.handleLifecycleChanged(ctx, env)

	case resEvent.TypePointCreated:
		return c.handlePointCreated(ctx, env)

	case resEvent.TypePointUpdated:
		return c.handlePointUpdated(ctx, env)

	case resEvent.TypePointDeleted:
		return c.handlePointDeleted(ctx, env)

	case resEvent.TypeCUUpdated:
		return c.handleCUUpdated(ctx, env)

	case resEvent.TypeImportCompleted:
		return c.handleImportCompleted(ctx, env)

	default:
		// Site, asset, and rename events do not change a CU's point bindings.
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

func (c *LifecycleConsumer) handleResourceDeleted(
	ctx context.Context,
	env platEvent.Envelope[json.RawMessage],
) error {
	var payload resEvent.ResourceDeletedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		logging.Warnf(ctx, logrus.Fields{
			"component": "LifecycleConsumer",
			"event_id":  env.EventID,
			"error":     err.Error(),
		}, "kafka: failed to deserialise ResourceDeletedPayload, skipping")
		return nil
	}

	tenantID := env.TenantID
	if tenantID == "" {
		tenantID = payload.TenantID
	}

	if payload.IncludeDescendants {
		c.invalidateTenant(tenantID)
	} else {
		c.invalidateCU(tenantID, payload.ResourceID)
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
		logging.Warnf(ctx, logrus.Fields{
			"component": "LifecycleConsumer",
			"event_id":  env.EventID,
			"error":     err.Error(),
		}, "kafka: failed to deserialise LifecycleChangedPayload, skipping")
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

	c.invalidateCU(tenantID, payload.ResourceID)

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

func (c *LifecycleConsumer) handlePointCreated(ctx context.Context, env platEvent.Envelope[json.RawMessage]) error {
	var payload resEvent.PointCreatedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.warnBadPayload(ctx, env.EventID, err)
		return nil
	}
	c.invalidateCUOrPoint(tenantIDOf(env.TenantID, payload.TenantID), payload.CUID, payload.PointID)
	return nil
}

func (c *LifecycleConsumer) handlePointUpdated(ctx context.Context, env platEvent.Envelope[json.RawMessage]) error {
	var payload resEvent.PointUpdatedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.warnBadPayload(ctx, env.EventID, err)
		return nil
	}
	c.invalidateCUOrPoint(tenantIDOf(env.TenantID, payload.TenantID), payload.CUID, payload.PointID)
	return nil
}

func (c *LifecycleConsumer) handlePointDeleted(ctx context.Context, env platEvent.Envelope[json.RawMessage]) error {
	var payload resEvent.PointDeletedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.warnBadPayload(ctx, env.EventID, err)
		return nil
	}
	c.invalidateCUOrPoint(tenantIDOf(env.TenantID, payload.TenantID), payload.CUID, payload.PointID)
	return nil
}

func (c *LifecycleConsumer) handleCUUpdated(ctx context.Context, env platEvent.Envelope[json.RawMessage]) error {
	var payload resEvent.CUUpdatedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.warnBadPayload(ctx, env.EventID, err)
		return nil
	}
	c.invalidateCU(tenantIDOf(env.TenantID, payload.TenantID), payload.CUID)
	return nil
}

func (c *LifecycleConsumer) handleImportCompleted(ctx context.Context, env platEvent.Envelope[json.RawMessage]) error {
	var payload resEvent.ImportCompletedPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		c.warnBadPayload(ctx, env.EventID, err)
		return nil
	}
	switch payload.TargetType {
	case "point", "cu":
		c.invalidateTenant(tenantIDOf(env.TenantID, payload.TenantID))
	}
	return nil
}

func (c *LifecycleConsumer) invalidateCUOrPoint(tenantID, cuID, pointID string) {
	if cuID != "" {
		c.invalidateCU(tenantID, cuID)
		return
	}
	c.invalidatePoint(tenantID, pointID)
}

func (c *LifecycleConsumer) invalidateCU(tenantID, cuID string) {
	if c.bindings == nil {
		return
	}
	c.bindings.InvalidateCU(tenantID, cuID)
}

func (c *LifecycleConsumer) invalidatePoint(tenantID, pointID string) {
	if c.bindings == nil {
		return
	}
	c.bindings.InvalidatePoint(tenantID, pointID)
}

func (c *LifecycleConsumer) invalidateTenant(tenantID string) {
	if c.bindings == nil {
		return
	}
	c.bindings.InvalidateTenant(tenantID)
}

func tenantIDOf(envelopeTenant, payloadTenant string) string {
	if envelopeTenant != "" {
		return envelopeTenant
	}
	return payloadTenant
}

func (c *LifecycleConsumer) warnBadPayload(ctx context.Context, eventID string, err error) {
	logging.Warnf(ctx, logrus.Fields{
		"component": "LifecycleConsumer",
		"event_id":  eventID,
		"error":     err.Error(),
	}, "kafka: failed to deserialise resource payload, skipping")
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
