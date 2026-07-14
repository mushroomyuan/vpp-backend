package telemetry

import (
	"context"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// KafkaConsumeInfo describes a consumed Kafka message for span attributes.
type KafkaConsumeInfo struct {
	Topic     string
	GroupID   string
	Key       string
	EventType string
	Partition int
	Offset    int64
}

// KafkaPublishInfo describes an outbound Kafka message for span attributes.
type KafkaPublishInfo struct {
	Topic     string
	Key       string
	EventType string
}

// StartKafkaConsumer extracts remote context from carrier (if any), then starts
// a CONSUMER span named "<topic> process" with messaging.* attributes.
func StartKafkaConsumer(ctx context.Context, carrier MapCarrier, info KafkaConsumeInfo) (context.Context, trace.Span) {
	if carrier != nil {
		ctx = Extract(ctx, carrier)
	}

	spanName := info.Topic + " process"
	if info.Topic == "" {
		spanName = "kafka process"
	}

	attrs := []attribute.KeyValue{
		semconv.MessagingSystemKafka,
		semconv.MessagingOperationTypeDeliver, // "process"
	}
	if info.Topic != "" {
		attrs = append(attrs, semconv.MessagingDestinationName(info.Topic))
	}
	if info.GroupID != "" {
		attrs = append(attrs, semconv.MessagingKafkaConsumerGroup(info.GroupID))
	}
	if info.Key != "" {
		attrs = append(attrs, semconv.MessagingKafkaMessageKey(info.Key))
	}
	if info.EventType != "" {
		attrs = append(attrs, attribute.String("messaging.message.type", info.EventType))
	}
	attrs = append(attrs,
		semconv.MessagingDestinationPartitionID(strconv.Itoa(info.Partition)),
		semconv.MessagingKafkaMessageOffset(int(info.Offset)),
	)

	return defaultTracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attrs...),
	)
}

// StartKafkaProducer starts a PRODUCER span named "<topic> publish".
// Call Inject on the returned ctx before writing Kafka headers so the
// consumer can continue this trace.
func StartKafkaProducer(ctx context.Context, info KafkaPublishInfo) (context.Context, trace.Span) {
	spanName := info.Topic + " publish"
	if info.Topic == "" {
		spanName = "kafka publish"
	}

	attrs := []attribute.KeyValue{
		semconv.MessagingSystemKafka,
		semconv.MessagingOperationTypePublish,
	}
	if info.Topic != "" {
		attrs = append(attrs, semconv.MessagingDestinationName(info.Topic))
	}
	if info.Key != "" {
		attrs = append(attrs, semconv.MessagingKafkaMessageKey(info.Key))
	}
	if info.EventType != "" {
		attrs = append(attrs, attribute.String("messaging.message.type", info.EventType))
	}

	return defaultTracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attrs...),
	)
}

// EndSpan ends the span, recording err when non-nil.
func EndSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// InjectToMap creates a MapCarrier with propagated context from ctx.
func InjectToMap(ctx context.Context) MapCarrier {
	c := make(MapCarrier, 4)
	Inject(ctx, c)
	return c
}
