package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type Config struct {
	// Example:
	// localhost:4318
	// jaeger:4318
	// tempo:4318
	Endpoint string

	// resource-service / order-service ...
	ServiceName string

	// Dev usually true
	Insecure bool
}

var defaultTracer = otel.Tracer("bootstrap")

// InitTracing initializes OpenTelemetry tracing.
//
// It is backend-agnostic:
// Jaeger / Tempo / Collector / other OTLP receivers.
//
// Call once during service startup.
func InitTracing(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("empty telemetry endpoint")
	}
	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("empty service name")
	}

	logrus.Infof(
		"init tracing service=%s endpoint=%s",
		cfg.ServiceName,
		cfg.Endpoint,
	)

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}

	res, err := resource.New(
		ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(
			exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	// Support modern W3C + legacy B3 propagation.
	b3Prop := b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader))

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
			b3Prop,
		),
	)

	defaultTracer = otel.Tracer(cfg.ServiceName)

	return tp.Shutdown, nil
}

// Tracer returns a named tracer.
func Tracer(name string) trace.Tracer {
	if name == "" {
		return defaultTracer
	}
	return otel.Tracer(name)
}

// Start starts a span using the default tracer.
func Start(ctx context.Context, spanName string) (context.Context, trace.Span) {
	return defaultTracer.Start(ctx, spanName)
}

// TraceID returns trace id from context.
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	return sc.TraceID().String()
}

// SpanID returns span id from context.
func SpanID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	return sc.SpanID().String()
}
