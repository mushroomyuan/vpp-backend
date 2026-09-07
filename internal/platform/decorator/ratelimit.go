package decorator

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/time/rate"
)

// ErrRateLimited is returned when a handler's rate limiter has no tokens
// left. Inbound adapters must map this to a "too many requests" transport
// status (gRPC codes.ResourceExhausted / HTTP 429).
var ErrRateLimited = errors.New("decorator: rate limit exceeded")

// WithRateLimit rejects a request once limiter.Allow() reports no tokens
// left, short-circuiting before Metrics/Tracing/the business handler run.
//
// A nil limiter disables rate limiting entirely (Allow is never called),
// which lets callers wire this middleware unconditionally and control it via
// config alone.
//
// One *rate.Limiter = one bucket for whatever scope the caller chose. v1 in
// this codebase uses one limiter per handler type (constructed at wiring
// time in each service's server.go), not per-tenant.
func WithRateLimit[C, R any](limiter *rate.Limiter, metricsClient MetricsClient) Middleware[C, R] {
	return func(next Handler[C, R]) Handler[C, R] {
		return handlerFunc[C, R](func(ctx context.Context, in C) (R, error) {
			if limiter != nil && !limiter.Allow() {
				if metricsClient != nil {
					action := strings.ToLower(generateActionName(in))
					metricsClient.Count("ratelimit", action, "rejected")
				}
				var zero R
				return zero, ErrRateLimited
			}
			return next.Handle(ctx, in)
		})
	}
}
