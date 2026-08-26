package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/sirupsen/logrus"

	"github.com/mushroomyuan/vpp-backend/alarm/application/command"
	"github.com/mushroomyuan/vpp-backend/alarm/domain/model"
	"github.com/mushroomyuan/vpp-backend/alarm/metrics"
	platEvent "github.com/mushroomyuan/vpp-backend/platform/event"
	dispEvent "github.com/mushroomyuan/vpp-backend/platform/event/dispatch"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

const DefaultDispatchGroupID = "vpp-alarm-dispatch-events"

type DispatchConsumerConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

type DispatchConsumer struct {
	cfg     DispatchConsumerConfig
	reader  *kafka.Reader
	handler command.IngestEventHandler
	metrics *metrics.Metrics
}

func NewDispatchConsumer(cfg DispatchConsumerConfig, handler command.IngestEventHandler, m *metrics.Metrics) *DispatchConsumer {
	if handler == nil {
		panic("NewDispatchConsumer: handler is required")
	}
	if cfg.Topic == "" {
		cfg.Topic = dispEvent.TopicDispatchEvents
	}
	if cfg.GroupID == "" {
		cfg.GroupID = DefaultDispatchGroupID
	}
	c := &DispatchConsumer{cfg: cfg, handler: handler, metrics: m}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — alarm dispatch consumer will not start")
		return c
	}
	c.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		CommitInterval: 0,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-alarm-dispatch] "+msg, args...)
		}),
	})
	logrus.Infof("kafka: alarm dispatch consumer initialised, brokers=%v topic=%s group=%s",
		cfg.Brokers, cfg.Topic, cfg.GroupID)
	return c
}

func (c *DispatchConsumer) Run(ctx context.Context) error {
	return runReader(ctx, c.reader, "AlarmDispatchConsumer", metrics.SourceDispatch, c.metrics, c.handleMessage)
}

func (c *DispatchConsumer) Close() error {
	if c.reader == nil {
		return nil
	}
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("kafka alarm dispatch consumer close: %w", err)
	}
	return nil
}

func (c *DispatchConsumer) handleMessage(ctx context.Context, msg kafka.Message) (class Class) {
	start := time.Now()
	defer func() {
		c.metrics.ObserveIngest(metrics.SourceDispatch, class.Result, class.Reason, time.Since(start))
	}()

	carrier := kafkaHeadersToCarrier(msg.Headers)
	ctx, span := plattelemetry.StartKafkaConsumer(ctx, carrier, plattelemetry.KafkaConsumeInfo{
		Topic:     msg.Topic,
		GroupID:   c.cfg.GroupID,
		Key:       string(msg.Key),
		EventType: peekEventType(msg.Value),
		Partition: msg.Partition,
		Offset:    msg.Offset,
	})
	defer func() {
		var spanErr error
		if !class.Commit {
			spanErr = fmt.Errorf("ingest %s/%s", class.Result, class.Reason)
		}
		plattelemetry.EndSpan(span, spanErr)
	}()

	var env platEvent.Envelope[json.RawMessage]
	if err := json.Unmarshal(msg.Value, &env); err != nil {
		logging.Warnf(ctx, logrus.Fields{
			"component": "AlarmDispatchConsumer",
			"error":     err.Error(),
		}, "kafka: poison dispatch envelope, committing skip")
		return DecodePoison()
	}
	if env.EventType != dispEvent.TypeTaskFailed {
		return Dropped()
	}

	var payload dispEvent.TaskLifecyclePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		logging.Warnf(ctx, logrus.Fields{
			"component": "AlarmDispatchConsumer",
			"error":     err.Error(),
		}, "kafka: poison task.failed payload, committing skip")
		return DecodePoison()
	}

	incoming := model.IncomingEvent{
		Source:     model.SourceDispatch,
		TenantID:   env.TenantID,
		EventID:    env.EventID,
		EventType:  env.EventType,
		OccurredAt: env.OccurredAt,
		TaskID:     payload.TaskID,
		TaskName:   payload.Name,
		TaskStatus: payload.Status,
	}
	if incoming.TenantID == "" {
		incoming.TenantID = payload.TenantID
	}

	res, err := c.handler.Handle(ctx, command.IngestEvent{Incoming: incoming})
	class = Classify(res, err)
	c.logClass(ctx, class, err)
	return class
}

func (c *DispatchConsumer) logClass(ctx context.Context, class Class, err error) {
	fields := logrus.Fields{
		"component": "AlarmDispatchConsumer",
		"result":    class.Result,
		"reason":    class.Reason,
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	switch class.Result {
	case ResultPoison:
		logging.Warnf(ctx, fields, "kafka: dispatch ingest poison, committing skip")
	case ResultRetry:
		logging.Warnf(ctx, fields, "kafka: dispatch ingest retry")
	}
}

func peekEventType(value []byte) string {
	var head struct {
		EventType string `json:"event_type"`
	}
	_ = json.Unmarshal(value, &head)
	return head.EventType
}
