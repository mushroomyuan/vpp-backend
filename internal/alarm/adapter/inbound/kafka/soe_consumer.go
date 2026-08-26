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
	telEvent "github.com/mushroomyuan/vpp-backend/platform/event/telemetry"
	"github.com/mushroomyuan/vpp-backend/platform/logging"
	plattelemetry "github.com/mushroomyuan/vpp-backend/platform/telemetry"
)

const DefaultSOEGroupID = "vpp-alarm-soe-events"

type SOEConsumerConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

type SOEConsumer struct {
	cfg     SOEConsumerConfig
	reader  *kafka.Reader
	handler command.IngestEventHandler
	metrics *metrics.Metrics
}

func NewSOEConsumer(cfg SOEConsumerConfig, handler command.IngestEventHandler, m *metrics.Metrics) *SOEConsumer {
	if handler == nil {
		panic("NewSOEConsumer: handler is required")
	}
	if cfg.Topic == "" {
		cfg.Topic = telEvent.TopicSOEEvents
	}
	if cfg.GroupID == "" {
		cfg.GroupID = DefaultSOEGroupID
	}
	c := &SOEConsumer{cfg: cfg, handler: handler, metrics: m}
	if len(cfg.Brokers) == 0 {
		logrus.Warn("kafka: no brokers configured — alarm SOE consumer will not start")
		return c
	}
	c.reader = kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.GroupID,
		CommitInterval: 0,
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...interface{}) {
			logrus.Errorf("[kafka-alarm-soe] "+msg, args...)
		}),
	})
	logrus.Infof("kafka: alarm SOE consumer initialised, brokers=%v topic=%s group=%s",
		cfg.Brokers, cfg.Topic, cfg.GroupID)
	return c
}

func (c *SOEConsumer) Run(ctx context.Context) error {
	return runReader(ctx, c.reader, "AlarmSOEConsumer", metrics.SourceSOE, c.metrics, c.handleMessage)
}

func (c *SOEConsumer) Close() error {
	if c.reader == nil {
		return nil
	}
	if err := c.reader.Close(); err != nil {
		return fmt.Errorf("kafka alarm SOE consumer close: %w", err)
	}
	return nil
}

func (c *SOEConsumer) handleMessage(ctx context.Context, msg kafka.Message) (class Class) {
	start := time.Now()
	defer func() {
		c.metrics.ObserveIngest(metrics.SourceSOE, class.Result, class.Reason, time.Since(start))
	}()

	carrier := kafkaHeadersToCarrier(msg.Headers)
	ctx, span := plattelemetry.StartKafkaConsumer(ctx, carrier, plattelemetry.KafkaConsumeInfo{
		Topic:     msg.Topic,
		GroupID:   c.cfg.GroupID,
		Key:       string(msg.Key),
		EventType: "soe",
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

	var payload telEvent.SOEPayload
	if err := json.Unmarshal(msg.Value, &payload); err != nil {
		logging.Warnf(ctx, logrus.Fields{
			"component": "AlarmSOEConsumer",
			"error":     err.Error(),
		}, "kafka: poison SOE payload, committing skip")
		return DecodePoison()
	}

	res, err := c.handler.Handle(ctx, command.IngestEvent{Incoming: model.IncomingEvent{
		Source:     model.SourceSOE,
		TenantID:   payload.TenantID,
		OccurredAt: payload.OccurredAt,
		CUCode:     payload.CUCode,
		MetricName: payload.MetricName,
		OldValue:   payload.OldValue,
		NewValue:   payload.NewValue,
	}})
	class = Classify(res, err)
	if class.Result == ResultPoison || class.Result == ResultRetry {
		fields := logrus.Fields{
			"component": "AlarmSOEConsumer",
			"result":    class.Result,
			"reason":    class.Reason,
		}
		if err != nil {
			fields["error"] = err.Error()
		}
		logging.Warnf(ctx, fields, "kafka: SOE ingest %s", class.Result)
	}
	return class
}
