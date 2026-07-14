package decorator

import (
	"context"
	"fmt"
	"strings"

	"github.com/mushroomyuan/vpp-backend/platform/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// WithTracing starts an application-layer span around the next handler.
// kind should be "command" or "query"; span name is "<kind>.<TypeName>".
func WithTracing[C, R any](kind string) Middleware[C, R] {
	return func(next Handler[C, R]) Handler[C, R] {
		return handlerFunc[C, R](func(ctx context.Context, in C) (result R, err error) {
			action := generateActionName(in)
			spanName := fmt.Sprintf("%s.%s", kind, action)
			ctx, span := telemetry.Start(ctx, spanName)
			defer span.End()

			span.SetAttributes(
				attribute.String("cqrs.kind", kind),
				attribute.String("cqrs.action", strings.ToLower(action)),
			)

			result, err = next.Handle(ctx, in)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			return result, err
		})
	}
}
