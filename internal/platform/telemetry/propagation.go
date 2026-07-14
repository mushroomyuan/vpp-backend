package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// MapCarrier is a propagation.TextMapCarrier backed by a string map.
// Use it to move W3C / B3 context through Kafka headers (or any string KV store).
type MapCarrier map[string]string

var _ propagation.TextMapCarrier = MapCarrier(nil)

func (c MapCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	return c[key]
}

func (c MapCarrier) Set(key, value string) {
	c[key] = value
}

func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// Inject writes the current span context into carrier (traceparent / baggage / B3).
func Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// Extract returns a context with remote parent span context from carrier.
// If carrier has no valid trace headers, the returned context is unchanged
// (a new root span can still be started from it).
func Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}
